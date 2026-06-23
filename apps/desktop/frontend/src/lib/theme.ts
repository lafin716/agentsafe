// Light/dark theme handling. Tailwind is configured with `darkMode: ["class"]`
// and index.css defines the `.dark` variable set, so switching themes is just a
// matter of toggling the `dark` class on <html>. Persisted to localStorage.

export type Theme = "light" | "dark";

const STORAGE_KEY = "agentsafe.theme";
const DEFAULT_THEME: Theme = "light";

export function getInitialTheme(): Theme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark") return stored;
  } catch {
    /* localStorage unavailable */
  }
  return DEFAULT_THEME;
}

export function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    /* localStorage unavailable */
  }
}
