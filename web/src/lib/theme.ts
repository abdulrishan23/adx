export type ThemePreference = "light" | "dark" | "system";
export type Theme = "light" | "dark";

export const THEME_STORAGE_KEY = "adx-theme";

const PREFERENCES: readonly ThemePreference[] = ["system", "light", "dark"];

// parsePreference turns whatever is stored into a known preference. Anything
// unknown, including nothing, means follow the system.
export function parsePreference(value: string | null | undefined): ThemePreference {
  return PREFERENCES.includes(value as ThemePreference) ? (value as ThemePreference) : "system";
}

export function resolveTheme(preference: ThemePreference, systemPrefersDark: boolean): Theme {
  if (preference === "system") {
    return systemPrefersDark ? "dark" : "light";
  }
  return preference;
}

// nextPreference is the order the toggle steps through.
export function nextPreference(current: ThemePreference): ThemePreference {
  const i = PREFERENCES.indexOf(current);
  return PREFERENCES[(i + 1) % PREFERENCES.length] ?? "system";
}
