import { useState } from "react";
import { usePeople, useUpdatePerson, useWriteActions } from "@/api/adx";
import type { PermissionMode, Person, WriteAction } from "@/api/models";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Sheet } from "@/components/ui/sheet";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

const ROLE_LABEL: Record<Person["role"], string> = {
  admin: "Admin",
  marketer: "Marketer",
  viewer: "Viewer",
};

const MODE_LABEL: Record<PermissionMode, string> = {
  allowed: "Allowed",
  approval: "Needs approval",
  off: "Off",
};

export function PeoplePage() {
  const people = usePeople();
  const actions = useWriteActions();
  const [openId, setOpenId] = useState<number | null>(null);
  const open = people.data?.find((p) => p.id === openId);

  return (
    <>
      <PageHeader title="People and permissions" />
      {(people.isPending || actions.isPending) && <LoadingRows rows={6} />}
      {people.error && <ErrorNote error={people.error} />}
      {actions.error && <ErrorNote error={actions.error} />}
      {people.data && actions.data && (
        <div className="space-y-6">
          <Panel bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>Person</TH>
                  <TH>Role</TH>
                  <TH>Overrides</TH>
                  <TH>Last active</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {people.data.map((p) => (
                  <TR key={p.id}>
                    <TD>
                      <div className="font-medium">{p.name}</div>
                      <div className="text-xs text-muted-foreground">{p.email}</div>
                    </TD>
                    <TD>
                      <RoleSelect person={p} />
                    </TD>
                    <TD className="text-muted-foreground">{overrideSummary(p, actions.data)}</TD>
                    <TD className="text-muted-foreground">
                      {p.lastActiveAt ? formatRelative(p.lastActiveAt) : "Never signed in"}
                    </TD>
                    <TD className="text-right">
                      {p.role === "marketer" && (
                        <Button size="sm" variant="outline" onClick={() => setOpenId(p.id)}>
                          Permissions
                        </Button>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>

          <Panel title="Defaults for marketers" bodyClassName="p-0">
            <ul className="divide-y">
              {actions.data.map((a) => (
                <li key={a.key} className="flex items-center justify-between gap-4 px-5 py-3">
                  <div>
                    <div className="text-sm font-medium">{a.label}</div>
                    <div className="text-xs text-muted-foreground">{a.detail}</div>
                  </div>
                  <ModeTag mode={a.defaultMode} />
                </li>
              ))}
            </ul>
          </Panel>
        </div>
      )}
      {open && actions.data && (
        <PermissionsSheet person={open} actions={actions.data} onClose={() => setOpenId(null)} />
      )}
    </>
  );
}

function overrideSummary(p: Person, actions: WriteAction[]) {
  if (p.role === "admin") return "Everything";
  if (p.role === "viewer") return "Read only";
  const keys = Object.keys(p.overrides);
  if (keys.length === 0) return "Defaults";
  return keys
    .map((k) => {
      const a = actions.find((x) => x.key === k);
      return `${a?.label ?? k}: ${MODE_LABEL[p.overrides[k] as PermissionMode].toLowerCase()}`;
    })
    .join(" · ");
}

function RoleSelect({ person }: { person: Person }) {
  const update = useUpdatePerson();
  return (
    <Select
      aria-label={`${person.name}'s role`}
      value={person.role}
      disabled={update.isPending}
      onValueChange={(role) => update.mutate({ id: person.id, role })}
      options={(Object.entries(ROLE_LABEL) as [Person["role"], string][]).map(([value, label]) => ({
        value,
        label,
      }))}
    />
  );
}

function ModeTag({ mode }: { mode: PermissionMode }) {
  return (
    <span
      className={cn(
        "inline-flex h-6 shrink-0 items-center rounded-sm border px-2 font-mono text-[0.65rem] tracking-wider uppercase",
        mode === "allowed" && "border-foreground/40 text-foreground",
        mode === "approval" && "text-muted-foreground",
        mode === "off" && "border-dashed text-muted-foreground",
      )}
    >
      {MODE_LABEL[mode]}
    </span>
  );
}

function PermissionsSheet({
  person,
  actions,
  onClose,
}: {
  person: Person;
  actions: WriteAction[];
  onClose: () => void;
}) {
  const update = useUpdatePerson();
  return (
    <Sheet open onOpenChange={(o) => !o && onClose()} title={person.name}>
      <ul className="divide-y">
        {actions.map((a) => {
          const override = person.overrides[a.key];
          const mode = override ?? a.defaultMode;
          return (
            <li key={a.key} className="space-y-2 py-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-medium">{a.label}</div>
                  <div className="text-xs text-muted-foreground">
                    {a.detail}
                    {override && ` · default is ${MODE_LABEL[a.defaultMode].toLowerCase()}`}
                  </div>
                </div>
                {override && (
                  <Button
                    size="xs"
                    variant="ghost"
                    disabled={update.isPending}
                    onClick={() =>
                      update.mutate({ id: person.id, override: { action: a.key, mode: null } })
                    }
                  >
                    Use default
                  </Button>
                )}
              </div>
              <Segmented
                label={a.label}
                value={mode}
                onChange={(m) =>
                  update.mutate({
                    id: person.id,
                    override: { action: a.key, mode: m === a.defaultMode ? null : m },
                  })
                }
                options={(["allowed", "approval", "off"] as const).map((m) => ({
                  value: m,
                  label: MODE_LABEL[m],
                }))}
              />
            </li>
          );
        })}
      </ul>
      {update.error && <ErrorNote error={update.error} />}
    </Sheet>
  );
}
