import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useContactability, useRequestCall } from "@/engine/api/calls";

// Places a qualification call for an enquiry, showing every reason it can't
// be placed first. Consent basis is typed by the person placing it.
export function CallButton({ enquiryId }: { enquiryId: number }) {
  const [consent, setConsent] = useState("");
  const ct = useContactability(enquiryId, consent);
  const request = useRequestCall();
  const blockers = ct.data?.blockers ?? [];
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-1">
        <Input
          className="h-7 max-w-64"
          placeholder="consent basis (where they agreed to a call)"
          value={consent}
          onChange={(e) => setConsent(e.target.value)}
        />
        <Button
          size="xs"
          variant="outline"
          disabled={blockers.length > 0 || request.isPending}
          onClick={() =>
            request.mutate({
              enquiryId,
              mode: "qualification",
              consentBasis: consent,
              reason: "qualification",
            })
          }
        >
          Call now
        </Button>
      </div>
      {blockers.length > 0 && (
        <p className="text-muted-foreground">Can't call: {blockers.join("; ")}</p>
      )}
      {request.data && (
        <p className="text-muted-foreground">
          Call #{request.data.id} is {request.data.state}.
        </p>
      )}
      {request.error && <p className="text-destructive">{request.error.message}</p>}
    </div>
  );
}
