import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/lib/theme";

// One 28px icon button: the icon shows the theme you would switch to, and
// the label says so in words.
export function ThemeToggle() {
  const [theme, toggle] = useTheme();
  const label = theme === "dark" ? "Switch to the light theme" : "Switch to the dark theme";
  const Icon = theme === "dark" ? Sun : Moon;
  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={label}
      title={label}
      className="flex size-7 shrink-0 items-center justify-center text-muted-foreground hover:bg-sidebar-accent hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
    >
      <Icon size={16} strokeWidth={2} aria-hidden="true" />
    </button>
  );
}
