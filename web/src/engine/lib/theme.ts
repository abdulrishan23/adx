import { useCallback, useSyncExternalStore } from "react";
import { readStored, writeStored } from "@/engine/lib/storage";

// Paper (light) is the default; Ink (dark) redefines the same tokens under
// .dark on <html> (design language, 03). The choice is remembered in the
// browser; with nothing remembered the system preference decides.
export type Theme = "light" | "dark";

const THEME_KEY = "adx.theme";

export function readTheme(): Theme {
  const stored = readStored(THEME_KEY);
  if (stored === "light" || stored === "dark") return stored;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

const listeners = new Set<() => void>();

function setTheme(theme: Theme): void {
  applyTheme(theme);
  writeStored(THEME_KEY, theme);
  for (const l of listeners) l();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function current(): Theme {
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

export function useTheme(): [Theme, () => void] {
  const theme = useSyncExternalStore<Theme>(subscribe, current, () => "light");
  const toggle = useCallback(() => setTheme(current() === "dark" ? "light" : "dark"), []);
  return [theme, toggle];
}
