// A preference the browser remembers between visits: the sidebar's width,
// the theme. localStorage can be missing or refused (private windows, blocked
// site data), so a failed read answers null and a failed write is dropped;
// the page must work either way.
export function readStored(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function writeStored(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // Nothing to do: the preference lasts for this page only.
  }
}
