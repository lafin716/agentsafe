// Popout bridge runtime.
//
// Wails v2 is single-window and does not inject `window.go` (bindings) or
// `window.runtime` (event bus) into windows opened with window.open. A detached
// "popout" window is served by the Go loopback bridge (apps/desktop/bridge.go)
// and reaches the same backend over HTTP RPC + Server-Sent Events. installBridge
// populates the two globals that `lib/api.ts` and `TerminalPanel.runtime()`
// read, so every existing page/component works unchanged inside a popout.

// getPopoutParams returns the detached view + token when the current document
// was opened as a popout (`/?popout=<viewJSON>&token=<token>`), else null.
export function getPopoutParams(): { view: string; token: string } | null {
  const params = new URLSearchParams(window.location.search);
  const view = params.get("popout");
  const token = params.get("token");
  if (!view || !token) return null;
  return { view, token };
}

type Handler = (...data: unknown[]) => void;

// installBridge wires window.go.main.App and window.runtime to the bridge using
// the given session token. Must run before the React app mounts.
export function installBridge(token: string): void {
  const auth = `token=${encodeURIComponent(token)}`;

  // Bindings: every App method call becomes POST /rpc/<Method> with a JSON
  // array body of args. Mirrors the Proxy in lib/api.ts so `api.*` just works.
  const app = new Proxy({} as Record<string, unknown>, {
    get(_target, prop: string) {
      return async (...args: unknown[]) => {
        const res = await fetch(`/rpc/${prop}?${auth}`, {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Bridge-Token": token },
          body: JSON.stringify(args),
        });
        if (!res.ok) {
          const msg = (await res.text()).trim();
          throw new Error(msg || `RPC ${prop} failed (${res.status})`);
        }
        const text = await res.text();
        return text ? JSON.parse(text) : undefined;
      };
    },
  });
  (window as unknown as { go: unknown }).go = { main: { App: app } };

  // Event bus mirrored from the backend over SSE. EventSource auto-reconnects.
  const handlers = new Map<string, Set<Handler>>();
  const es = new EventSource(`/events?${auth}`);
  es.onmessage = (e) => {
    try {
      const { event, data } = JSON.parse(e.data) as { event: string; data: unknown };
      const set = handlers.get(event);
      if (set) for (const cb of set) cb(data);
    } catch {
      /* ignore malformed event */
    }
  };

  (window as unknown as { runtime: unknown }).runtime = {
    EventsOn(event: string, cb: Handler) {
      let set = handlers.get(event);
      if (!set) {
        set = new Set<Handler>();
        handlers.set(event, set);
      }
      set.add(cb);
      return () => {
        set?.delete(cb);
      };
    },
    // Popout windows consume events but do not emit them back to the backend.
    EventsEmit() {
      /* no-op */
    },
  };
}
