import { signInUrl, useSession, useSignOut } from "@/api/session";
import { Button } from "@/components/ui/button";

// Header account state: checking, signed out (link to sign in), the dev
// admin (clearly labelled), or the person with their role and a sign-out.
export function AccountMenu() {
  const session = useSession();
  const signOut = useSignOut();

  if (session.isPending) {
    return <span className="text-sm text-muted-foreground">…</span>;
  }
  if (session.error) {
    return <span className="text-sm text-destructive">Can’t reach the server</span>;
  }
  if (session.data === null) {
    const here = `${window.location.pathname}${window.location.search}`;
    return (
      <a href={signInUrl(here)} className="text-sm underline underline-offset-4">
        Sign in
      </a>
    );
  }
  const s = session.data;
  const who = s.dev ? "Dev admin (no sign-in)" : (s.user?.name ?? s.user?.email ?? "Signed in");
  return (
    <div className="flex items-center gap-3 text-sm">
      <span className="text-muted-foreground">
        {who} · {s.role} · {s.workspace}
      </span>
      {!s.dev && (
        <Button
          variant="outline"
          size="sm"
          onClick={() =>
            signOut.mutate(undefined, { onSuccess: () => window.location.assign("/") })
          }
          disabled={signOut.isPending}
        >
          Sign out
        </Button>
      )}
    </div>
  );
}
