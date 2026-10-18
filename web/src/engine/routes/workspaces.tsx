import { useEffect, useState } from "react";
import type { IndustryDocument, Role } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useSession } from "@/engine/api/session";
import {
  useAddMember,
  useCreateWorkspace,
  useIndustry,
  useMembers,
  usePutIndustry,
  useRemoveMember,
  useWorkspaces,
} from "@/engine/api/workspaces";

// Workspaces: which clients exist, who works in this one, and what this
// client's business looks like to ADX. Everything here is per workspace;
// another client's rows never appear.
export function WorkspacesPage() {
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const platformAdmin = isAdmin && session.data?.workspaceId === 1;
  return (
    <div className="space-y-10">
      <section className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">Workspaces</h1>
        <p className="text-sm text-muted-foreground">
          Each client is a workspace with its own accounts, projects, people and history. You are in{" "}
          <strong>{session.data?.workspace ?? "…"}</strong>.
        </p>
        <WorkspaceList />
        {platformAdmin && <NewWorkspace />}
      </section>
      {isAdmin && <Members />}
      <Industry canEdit={isAdmin} />
    </div>
  );
}

function WorkspaceList() {
  const list = useWorkspaces();
  if (list.isPending) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (list.error) return <p className="text-sm text-destructive">{list.error.message}</p>;
  return (
    <ul className="divide-y rounded border">
      {list.data.workspaces.map((w) => (
        <li key={w.id} className="flex items-center justify-between px-3 py-2 text-sm">
          <span>
            {w.name} <span className="text-muted-foreground">({w.slug})</span>
          </span>
          <span className="text-muted-foreground">
            {w.role}
            {w.current ? " · current" : ""}
          </span>
        </li>
      ))}
    </ul>
  );
}

function NewWorkspace() {
  const create = useCreateWorkspace();
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [industry, setIndustry] = useState("");
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { slug, name, industry },
          {
            onSuccess: () => {
              setSlug("");
              setName("");
              setIndustry("");
            },
          },
        );
      }}
    >
      <label className="text-sm" htmlFor="ws-slug">
        Slug
        <Input
          id="ws-slug"
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          placeholder="acme-homes"
        />
      </label>
      <label className="text-sm" htmlFor="ws-name">
        Name
        <Input
          id="ws-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Acme Homes"
        />
      </label>
      <label className="text-sm" htmlFor="ws-industry">
        Industry
        <Input
          id="ws-industry"
          value={industry}
          onChange={(e) => setIndustry(e.target.value)}
          placeholder="real_estate"
        />
      </label>
      <Button type="submit" disabled={create.isPending || !slug || !name}>
        Add client
      </Button>
      {create.error && <p className="w-full text-sm text-destructive">{create.error.message}</p>}
    </form>
  );
}

function Members() {
  const members = useMembers();
  const add = useAddMember();
  const remove = useRemoveMember();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("viewer");
  return (
    <section className="space-y-3">
      <h2 className="text-2xl font-semibold tracking-tight">Members</h2>
      <p className="text-sm text-muted-foreground">
        People who may act in this workspace. Someone must exist in ADX before they can be added
        here.
      </p>
      {members.data && (
        <ul className="divide-y rounded border">
          {members.data.members.map((m) => (
            <li key={m.userId} className="flex items-center justify-between px-3 py-2 text-sm">
              <span>
                {m.name ?? m.email}{" "}
                <span className="text-muted-foreground">
                  · {m.role}
                  {m.enabled ? "" : " · disabled"}
                </span>
              </span>
              {m.enabled && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => remove.mutate(m.userId)}
                  disabled={remove.isPending}
                >
                  Remove
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate({ email, role }, { onSuccess: () => setEmail("") });
        }}
      >
        <label className="text-sm" htmlFor="member-email">
          Email
          <Input
            id="member-email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="name@client.com"
          />
        </label>
        <label className="text-sm" htmlFor="member-role">
          Role
          <select
            id="member-role"
            className="block rounded border px-2 py-1"
            value={role}
            onChange={(e) => setRole(e.target.value as Role)}
          >
            <option value="viewer">viewer</option>
            <option value="marketer">marketer</option>
            <option value="admin">admin</option>
          </select>
        </label>
        <Button type="submit" disabled={add.isPending || !email}>
          Add member
        </Button>
        {(add.error || remove.error) && (
          <p className="w-full text-sm text-destructive">{(add.error ?? remove.error)?.message}</p>
        )}
      </form>
    </section>
  );
}

function Industry({ canEdit }: { canEdit: boolean }) {
  const industry = useIndustry();
  const put = usePutIndustry();
  const [text, setText] = useState("");
  useEffect(() => {
    if (industry.data) {
      setText(
        JSON.stringify(
          industry.data.current?.document ?? {
            industry: "",
            vocabulary: {},
            stages: [],
            qualifiers: [],
            notes: "",
          },
          null,
          2,
        ),
      );
    }
  }, [industry.data]);
  function save() {
    let doc: IndustryDocument;
    try {
      doc = JSON.parse(text) as IndustryDocument;
    } catch {
      return;
    }
    put.mutate(doc);
  }
  return (
    <section className="space-y-3">
      <h2 className="text-2xl font-semibold tracking-tight">Industry configuration</h2>
      <p className="text-sm text-muted-foreground">
        What this client's business looks like to ADX: vocabulary, stages and qualifiers. Saving
        writes a new version; earlier versions stay as they were.{" "}
        {industry.data?.current
          ? `Current version ${industry.data.current.version} of ${industry.data.versions.length}.`
          : "No configuration yet."}
      </p>
      <textarea
        className="h-64 w-full rounded border p-2 font-mono text-xs"
        value={text}
        onChange={(e) => setText(e.target.value)}
        readOnly={!canEdit}
      />
      {canEdit && (
        <Button onClick={save} disabled={put.isPending}>
          Save as new version
        </Button>
      )}
      {put.error && <p className="text-sm text-destructive">{put.error.message}</p>}
    </section>
  );
}
