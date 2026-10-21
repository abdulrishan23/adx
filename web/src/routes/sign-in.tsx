import { ArrowLeft, Mail } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import { AdxMark } from "@/components/brand";
import { Form } from "@/components/form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// A placeholder from before sign-in was designed. Real sign-in is authentik's
// login page (D-24), so this screen goes once the redirect is built.
export function SignInPage() {
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const id = useId();

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const value = email.trim();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) {
      setError("Enter a full email address");
      return;
    }
    setError(null);
    setSentTo(value);
  }

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-background px-4">
      <div className="w-full max-w-sm">
        <AdxMark className="mb-8 size-8" />
        {sentTo ? (
          <div className="space-y-4">
            <Mail className="size-6" aria-hidden="true" />
            <h1 className="text-2xl font-semibold tracking-tight">Check your email</h1>
            <p className="text-muted-foreground">
              A sign-in link is on its way to <span className="text-foreground">{sentTo}</span>. It
              works once, for 15 minutes.
            </p>
            <Button variant="ghost" size="sm" className="-ml-2" onClick={() => setSentTo(null)}>
              <ArrowLeft /> Use another email
            </Button>
          </div>
        ) : (
          <Form onSubmit={onSubmit} className="space-y-4">
            <h1 className="text-2xl font-semibold tracking-tight">Sign in to Adx</h1>
            <div className="space-y-1.5">
              <label htmlFor={id} className="text-sm">
                Work email
              </label>
              <Input
                id={id}
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => {
                  setError(null);
                  setEmail(e.target.value);
                }}
                aria-invalid={error !== null}
                className="h-10"
              />
              {error && <p className="text-sm text-destructive">{error}</p>}
            </div>
            <Button type="submit" className="h-10 w-full">
              Email me a sign-in link
            </Button>
          </Form>
        )}
      </div>
    </div>
  );
}
