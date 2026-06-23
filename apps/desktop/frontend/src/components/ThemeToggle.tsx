import { useState } from "react";
import { Moon, Sun } from "lucide-react";
import { cn } from "@/lib/utils";
import { useI18n } from "@/i18n/I18nProvider";
import { applyTheme, getInitialTheme, type Theme } from "@/lib/theme";

// A left-right sliding switch that toggles light/dark mode. The track shows a
// sun (left, light) and moon (right, dark); the knob slides to the active side.
export function ThemeToggle() {
  const { t } = useI18n();
  const [theme, setTheme] = useState<Theme>(getInitialTheme);
  const isDark = theme === "dark";

  function toggle() {
    const next: Theme = isDark ? "light" : "dark";
    applyTheme(next);
    setTheme(next);
  }

  const label = t(isDark ? "theme.switchToLight" : "theme.switchToDark");

  return (
    <button
      type="button"
      role="switch"
      aria-checked={isDark}
      onClick={toggle}
      title={label}
      aria-label={label}
      className="relative inline-flex h-7 w-14 shrink-0 items-center rounded-full border bg-muted transition-colors hover:bg-accent"
    >
      <Sun className="absolute left-1.5 size-3.5 text-amber-500" />
      <Moon className="absolute right-1.5 size-3.5 text-slate-400" />
      <span
        className={cn(
          "relative z-10 flex size-5 items-center justify-center rounded-full bg-background shadow-sm transition-transform duration-200 ease-in-out",
          isDark ? "translate-x-8" : "translate-x-1"
        )}
      >
        {isDark ? (
          <Moon className="size-3 text-foreground" />
        ) : (
          <Sun className="size-3 text-amber-500" />
        )}
      </span>
    </button>
  );
}
