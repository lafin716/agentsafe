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
    const term = new XTerm({
      cursorBlink: true,
      convertEol: true,
      fontFamily: "Menlo, Monaco, Consolas, monospace",
      fontSize: 12,
      theme: { background: "#0f172a", foreground: "#e2e8f0" },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(container);
    term.focus();
    termRef.current = term;

    const fit = () => {
      try {
        fitAddon.fit();
        void api.TerminalResize(id, term.cols, term.rows);
      } catch {
        /* element may be hidden during layout */
      }
    };
    const writeDisposable = term.onData((data) => {
      void api.TerminalWrite(id, data);
    });
    const rt = runtime();
    const offData = rt?.EventsOn("terminal:data", (...data: unknown[]) => {
      const payload = data[0] as { id: string; data: string };
      if (payload.id === id) term.write(payload.data);
    });
    const offClose = rt?.EventsOn("terminal:close", (...data: unknown[]) => {
      const payload = data[0] as { id: string; error?: string };
      if (payload.id === id) {
        term.writeln("");
        term.writeln(payload.error ? `[closed] ${payload.error}` : "[closed]");
      }
    });
    const resizeObserver = new ResizeObserver(fit);
    resizeObserver.observe(container);
    window.setTimeout(fit, 0);

    return () => {
      writeDisposable.dispose();
      resizeObserver.disconnect();
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
      <div ref={containerRef} className="min-h-0 flex-1 bg-slate-950 p-2" />
    </div>
  );
}
