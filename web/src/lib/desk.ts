// Copy for the home desk, kept pure so the wording is tested: short
// sentences, the unit with the number, "Not available" never 0 for a
// missing figure (design language, 11).

export function greeting(hour: number, name: string | null | undefined): string {
  const time = hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  const first = name?.trim().split(/\s+/)[0];
  return first ? `${time}, ${first}` : time;
}

export function plural(n: number, one: string, many: string): string {
  return `${n.toLocaleString("en")} ${n === 1 ? one : many}`;
}

// "1 thing needs your attention. 2 jobs under way. Server ready."
export function summaryLine(input: {
  waiting: number | null;
  openAlerts: number | null;
  jobs: number | null;
  server: "ready" | "not ready" | "unreachable" | "checking";
  failedChecks?: string[];
}): string {
  const parts: string[] = [];
  if (input.waiting === null && input.openAlerts === null) {
    parts.push("Attention count not available.");
  } else {
    const n = (input.waiting ?? 0) + (input.openAlerts ?? 0);
    parts.push(
      n === 0
        ? "Nothing needs your attention."
        : `${plural(n, "thing needs", "things need")} your attention.`,
    );
  }
  if (input.jobs === null) parts.push("Jobs not available.");
  else
    parts.push(
      input.jobs === 0 ? "No jobs under way." : `${plural(input.jobs, "job", "jobs")} under way.`,
    );
  switch (input.server) {
    case "ready":
      parts.push("Server ready.");
      break;
    case "not ready":
      parts.push(
        input.failedChecks && input.failedChecks.length > 0
          ? `Server not ready: ${input.failedChecks.join(", ")}.`
          : "Server not ready.",
      );
      break;
    case "unreachable":
      parts.push("Server unreachable.");
      break;
    default:
      parts.push("Checking the server.");
  }
  return parts.join(" ");
}

// Days back from today as YYYY-MM-DD, for the 30-day window the cards read.
export function dayAgo(days: number, now = new Date()): string {
  const d = new Date(now);
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}
