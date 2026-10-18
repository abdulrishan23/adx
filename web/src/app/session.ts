import type { Role } from "@/api/types.legacy";

// useSession says who is signed in. Sign-in isn't built yet (D-24), and the
// dev server treats every request as an admin (server/api/actor.go), so this
// mirrors that. Sign-in replaces this hook and nothing else.
export type Session = { name: string; email: string; role: Role };

export function useSession(): Session {
  return { name: "Abdul", email: "admin@localhost", role: "admin" };
}

export const ROLE_LABEL: Record<Role, string> = {
  admin: "Admin",
  marketer: "Marketer",
  viewer: "Viewer",
};

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const first = parts[0]?.[0] ?? "?";
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return (first + last).toUpperCase();
}
