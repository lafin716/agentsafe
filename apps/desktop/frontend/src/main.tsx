import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { ToastProvider } from "./components/ui/toast";
import { ConfirmProvider } from "./components/ui/confirm";
import { TaskProgress } from "./components/ui/task-progress";
import {
  LogConsoleProvider,
  LogConsoleWindow,
} from "./components/ui/log-console";
import { I18nProvider } from "./i18n/I18nProvider";
import { applyTheme, getInitialTheme } from "./lib/theme";
import "./index.css";

applyTheme(getInitialTheme());

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <I18nProvider>
      <ToastProvider>
        <ConfirmProvider>
          <LogConsoleProvider>
            <App />
            <TaskProgress />
            <LogConsoleWindow />
          </LogConsoleProvider>
        </ConfirmProvider>
      </ToastProvider>
    </I18nProvider>
  </React.StrictMode>
);
