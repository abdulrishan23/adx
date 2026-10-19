import { useState } from "react";
import { useRotateReviewKey, useUpdateCreativeSettings } from "@/api/creative";
import type { Settings } from "@/api/types.gen";
import { Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { reviewLink } from "@/routes/creative-project";
import { Field } from "./ui";

export function SettingsTab({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const save = useUpdateCreativeSettings(settings.projectId);
  const rotate = useRotateReviewKey(settings.projectId);
  const [prefix, setPrefix] = useState(settings.prefix);
  const [kicker, setKicker] = useState(settings.kicker);
  const [pageName, setPageName] = useState(settings.pageName);
  const dirty =
    prefix !== settings.prefix || kicker !== settings.kicker || pageName !== settings.pageName;

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Panel
        title="Names"
        description="How files are named and what the review board says at the top."
      >
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate({ prefix, kicker, pageName, reviewActive: settings.reviewActive });
          }}
        >
          <Field
            label="File prefix"
            htmlFor="prefix"
            hint={`The start of every file name, such as ${prefix || "MARLIN"}_C1_CASH_EN_9x16.jpg. It's fixed once the project has ads.`}
          >
            <Input
              id="prefix"
              value={prefix}
              disabled={!canEdit}
              onChange={(e) => setPrefix(e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, ""))}
              className="font-mono"
            />
          </Field>
          <Field
            label="Kicker"
            htmlFor="kicker"
            hint="The small line above the board's title, such as Meta ads · October 2026."
          >
            <Input
              id="kicker"
              value={kicker}
              disabled={!canEdit}
              onChange={(e) => setKicker(e.target.value)}
            />
          </Field>
          <Field
            label="Page name"
            htmlFor="page-name"
            hint="The Facebook or Instagram page shown in the phone frames."
          >
            <Input
              id="page-name"
              value={pageName}
              disabled={!canEdit}
              onChange={(e) => setPageName(e.target.value)}
            />
          </Field>
          {canEdit && (
            <div className="flex items-center gap-3">
              <Button type="submit" disabled={!dirty || save.isPending}>
                {save.isPending ? "Saving…" : "Save"}
              </Button>
              {save.error && <p className="text-sm text-destructive">{save.error.message}</p>}
              {save.isSuccess && !dirty && <p className="text-sm text-muted-foreground">Saved.</p>}
            </div>
          )}
        </form>
      </Panel>
      <Panel
        title="Review link"
        description="Anyone with the link can review without signing in. Send it to the review group only."
      >
        <div className="space-y-4">
          <p className="break-all font-mono text-sm">{reviewLink(settings.reviewKey)}</p>
          <p className="text-sm">
            {settings.reviewActive
              ? "The link is open."
              : "The link is paused. Reviewers see that it isn't active until you open it again. Nothing is deleted."}
          </p>
          {canEdit && (
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                disabled={save.isPending}
                onClick={() =>
                  save.mutate({
                    prefix: settings.prefix,
                    kicker: settings.kicker,
                    pageName: settings.pageName,
                    reviewActive: !settings.reviewActive,
                  })
                }
              >
                {settings.reviewActive ? "Pause the link" : "Open the link again"}
              </Button>
              <Button
                variant="ghost"
                disabled={rotate.isPending}
                onClick={() => {
                  if (
                    window.confirm("Make a new link? The current one stops working straight away.")
                  ) {
                    rotate.mutate();
                  }
                }}
              >
                Make a new link
              </Button>
            </div>
          )}
          {rotate.error && <p className="text-sm text-destructive">{rotate.error.message}</p>}
        </div>
      </Panel>
    </div>
  );
}
