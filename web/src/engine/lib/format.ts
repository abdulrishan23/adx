// Shown wherever a figure is missing. ADX never shows a missing figure as 0.
export const NOT_AVAILABLE = "Not available";

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
