import React from "react";
import ReactDOM from "react-dom/client";
import App, { type View } from "./App";
import { PopoutHost } from "./pages/PopoutHost";
import { ToastProvider } from "./components/ui/toast";
import { ConfirmProvider } from "./components/ui/confirm";
import { TaskProgress } from "./components/ui/task-progress";
import {
  LogConsoleProvider,
  LogConsoleWindow,
} from "./components/ui/log-console";
import { I18nProvider } from "./i18n/I18nProvider";
import { applyTheme, getInitialTheme } from "./lib/theme";
import { getPopoutParams, installBridge } from "./lib/bridge";
import "./index.css";

applyTheme(getInitialTheme());

// When this document was opened as a detached popout window, wire the bridge
// (window.go + window.runtime over the loopback server) before mounting, then
// render just the detached view instead of the full app shell.
const popout = getPopoutParams();
let rootNode: React.ReactNode;
if (popout) {
  installBridge(popout.token);
  let view: View | null = null;
  try {
    view = JSON.parse(popout.view) as View;
  } catch {
    view = null;
  }
  rootNode = view ? (
    <PopoutHost initial={view} />
  ) : (
    <div style={{ padding: 24, fontFamily: "sans-serif" }}>Invalid popout view.</div>
  );
} else {
  rootNode = (
    <>
      <App />
      <TaskProgress />
      <LogConsoleWindow />
    </>
  );
}

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <LogConsoleProvider>{rootNode}</LogConsoleProvider>
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  </React.StrictMode>
);
