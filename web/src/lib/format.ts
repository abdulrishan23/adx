// Shown wherever a figure is missing. ADX never shows a missing figure as 0.
export const NOT_AVAILABLE = "Not available";

// The reporting day is Dubai time (CLAUDE.md).
export const TIME_ZONE = "Asia/Dubai";

// formatMoney takes an amount in minor units, as the API sends it. The
// currency decides how many minor units make one major unit.
export function formatMoney(minor: number | null | undefined, currency: string): string {
  if (minor === null || minor === undefined) {
    return NOT_AVAILABLE;
  }
  const format = new Intl.NumberFormat("en", {
    style: "currency",
    currency,
    currencyDisplay: "code",
  });
  const digits = format.resolvedOptions().maximumFractionDigits ?? 2;
  return format.format(minor / 10 ** digits);
}

// formatMoneyShort is for tiles and tables: whole units, compact above 10,000.
// AED has 2 minor digits, which is the only currency Adx uses today.
export function formatMoneyShort(minor: number | null | undefined, currency = "AED"): string {
  if (minor === null || minor === undefined) {
    return NOT_AVAILABLE;
  }
  const major = minor / 100;
  const value = new Intl.NumberFormat("en", {
    notation: Math.abs(major) >= 10_000 ? "compact" : "standard",
    maximumFractionDigits: Math.abs(major) >= 10_000 ? 1 : 0,
  }).format(major);
  return `${currency} ${value}`;
}

export function formatNumber(n: number | null | undefined): string {
  if (n === null || n === undefined) {
    return NOT_AVAILABLE;
  }
  return new Intl.NumberFormat("en").format(n);
}

export function formatPercent(ratio: number | null | undefined, digits = 0): string {
  if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
    return NOT_AVAILABLE;
  }
  return new Intl.NumberFormat("en", {
    style: "percent",
    maximumFractionDigits: digits,
  }).format(ratio);
}

export type Change = { ratio: number; direction: "up" | "down" | "flat" };

// change compares a figure with the one before it. Null when either side is
// missing or the earlier figure is 0, since a percentage from 0 means nothing.
export function change(current: number | null, previous: number | null): Change | null {
  if (current === null || previous === null || previous === 0) {
    return null;
  }
  const ratio = (current - previous) / previous;
  const direction = Math.abs(ratio) < 0.005 ? "flat" : ratio > 0 ? "up" : "down";
  return { ratio, direction };
}

export function formatChange(c: Change | null): string {
  if (!c) {
    return "No earlier period";
  }
  if (c.direction === "flat") {
    return "No change";
  }
  const sign = c.ratio > 0 ? "+" : "−";
  return `${sign}${formatPercent(Math.abs(c.ratio))}`;
}

export function formatRelative(iso: string, now: Date = new Date()): string {
  const diff = now.getTime() - Date.parse(iso);
  const future = diff < 0;
  const mins = Math.round(Math.abs(diff) / 60_000);
  let text: string;
  if (mins < 1) text = "just now";
  else if (mins < 60) text = `${mins}m`;
  else if (mins < 60 * 24) text = `${Math.round(mins / 60)}h`;
  else text = `${Math.round(mins / 1440)}d`;
  if (text === "just now") return text;
  return future ? `in ${text}` : `${text} ago`;
}

export function formatDate(
  iso: string,
  opts: Intl.DateTimeFormatOptions = { day: "numeric", month: "short" },
): string {
  return new Intl.DateTimeFormat("en-GB", { timeZone: TIME_ZONE, ...opts }).format(new Date(iso));
}

// Date and time in Dubai, for when a day isn't enough: a deployment, a
// submission.
export function formatDateTime(iso: string): string {
  return formatDate(iso, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}
