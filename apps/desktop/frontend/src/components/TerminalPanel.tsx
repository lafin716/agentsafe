import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { api } from "@/lib/api";

export type WailsRuntime = {
  EventsOn: (event: string, cb: (...data: unknown[]) => void) => () => void;
};

// runtime exposes the Wails event bus injected on window. Returns null when the
// app is not running inside Wails (e.g. plain browser dev).
export function runtime(): WailsRuntime | null {
  const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
  return rt && typeof rt.EventsOn === "function" ? rt : null;
}

// TerminalPanel renders an xterm bound to a backend pty session (by id). It is
// shared by the file explorer and the feature work tab's agent run.
export function TerminalPanel({
  id,
  path,
  className,
}: {
  id: string;
  path: string;
  className?: string;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<XTerm | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const isWindows = navigator.userAgent.includes("Windows");
    const term = new XTerm({
      cursorBlink: true,
      convertEol: false,
      scrollback: 5000,
      fontFamily:
        '"Cascadia Mono", "Cascadia Code", Consolas, "Lucida Console", Menlo, Monaco, monospace',
      fontSize: 14,
      letterSpacing: 0.2,
      lineHeight: 1.2,
      allowTransparency: false,
      customGlyphs: true,
      drawBoldTextInBrightColors: true,
      ...(isWindows ? { windowsPty: { backend: "conpty" as const } } : {}),
      theme: { background: "#0f172a", foreground: "#e2e8f0" },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    term.focus();
    termRef.current = term;

    const fit = () => {
      try {
        if (!container.isConnected || container.clientWidth <= 0 || container.clientHeight <= 0) {
          return;
        }
        fitAddon.fit();
        void api.TerminalResize(id, term.cols, term.rows);
      } catch {
        /* element may be hidden during layout */
      }
    };
    const queueFit = () => {
      window.requestAnimationFrame(() => {
        fit();
        window.setTimeout(fit, 50);
      });
    };
    const rt = runtime();
    let disposed = false;
    let snapshotLoaded = false;
    let lastSeq = 0;
    let closedWritten = false;
    const pending: Array<{ data: string; seq?: number }> = [];
    let pendingClose: { error?: string } | null = null;
    term.attachCustomKeyEventHandler((event) => {
      if (event.type !== "keydown" || !event.ctrlKey || event.altKey || event.metaKey) {
        return true;
      }
      const key = event.key.toLowerCase();
      if (key === "c" && term.hasSelection()) {
        event.preventDefault();
        event.stopPropagation();
        void navigator.clipboard?.writeText?.(term.getSelection());
        return false;
      }
      if (key === "v") {
        event.preventDefault();
        event.stopPropagation();
        const clipboard = navigator.clipboard;
        if (!closedWritten && clipboard) {
          void clipboard.readText().then((text) => {
            if (text) void api.TerminalWrite(id, text);
          });
        }
        return false;
      }
      return true;
    });
    const pasteHandler = (event: ClipboardEvent) => {
      if (closedWritten) return;
      const text = event.clipboardData?.getData("text/plain");
      if (!text) return;
      event.preventDefault();
      void api.TerminalWrite(id, text);
    };
    container.addEventListener("paste", pasteHandler);
    const writeDisposable = term.onData((data) => {
      if (closedWritten) return;
      void api.TerminalWrite(id, data);
    });
    const writeClosed = (error?: string) => {
      if (closedWritten) return;
      closedWritten = true;
      term.writeln("");
      term.writeln(error ? `[closed] ${error}` : "[closed]");
    };
    const writePayload = (payload: { data: string; seq?: number }) => {
      if (typeof payload.seq === "number") {
        if (payload.seq <= lastSeq) return;
        lastSeq = payload.seq;
      }
      term.write(payload.data);
    };
    const offData = rt?.EventsOn("terminal:data", (...data: unknown[]) => {
      const payload = data[0] as { id: string; data: string; seq?: number };
      if (payload.id !== id) return;
      const next = { data: payload.data, seq: payload.seq };
      if (!snapshotLoaded) {
        pending.push(next);
        return;
      }
      writePayload(next);
    });
    const offClose = rt?.EventsOn("terminal:close", (...data: unknown[]) => {
      const payload = data[0] as { id: string; error?: string };
      if (payload.id === id) {
        if (!snapshotLoaded) {
          pendingClose = { error: payload.error };
          return;
        }
        writeClosed(payload.error);
      }
    });
    const restoreSnapshot = async () => {
      try {
        const snapshot = await api.TerminalSnapshot(id);
        if (disposed) return;
        if (snapshot.data) term.write(snapshot.data);
        lastSeq = snapshot.seq ?? 0;
        snapshotLoaded = true;
        for (const item of pending.splice(0)) writePayload(item);
        if (snapshot.closed) writeClosed(snapshot.error);
        else if (pendingClose) writeClosed(pendingClose.error);
      } catch {
        if (disposed) return;
        snapshotLoaded = true;
        for (const item of pending.splice(0)) writePayload(item);
        if (pendingClose) writeClosed(pendingClose.error);
      }
    };
    void restoreSnapshot();
    const resizeObserver = new ResizeObserver(fit);
    resizeObserver.observe(container);
    queueFit();
    window.setTimeout(fit, 250);
    void document.fonts?.ready.then(() => {
      if (!disposed) queueFit();
    });

    return () => {
      disposed = true;
      writeDisposable.dispose();
      resizeObserver.disconnect();
      container.removeEventListener("paste", pasteHandler);
      offData?.();
      offClose?.();
      term.dispose();
      termRef.current = null;
    };
  }, [id]);

  return (
    <div className={className ?? "flex h-[calc(100vh-12rem)] flex-col"}>
      <div className="border-b px-3 py-2 font-mono text-xs text-muted-foreground">
        {path}
      </div>
      <div ref={containerRef} className="min-h-0 flex-1 overflow-hidden bg-slate-950 p-2" />
    </div>
  );
}
