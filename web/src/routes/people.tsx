import { type FormEvent, useState } from "react";
import {
  useCandidates,
  useCreatePerson,
  useDecideCandidate,
  useEnrichPerson,
  usePeople,
  usePerson,
  useReverseLink,
  useSetStated,
  useSyncCRM,
  useUnmergePerson,
} from "@/api/crm";
import { useProjects } from "@/api/projects";
import { useSession } from "@/api/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { BriefPanel } from "@/routes/brief";
import { ScoreSection } from "@/routes/person-score";

// People and their enquiries. A person is who; an enquiry is a project
// they asked about. Matching is conservative and every uncertain match is
// shown for a person to decide; links and merges can be undone; what the
// customer stated is never overwritten by a provider's guess.
export function PeoplePage() {
  const [q, setQ] = useState("");
  const [current, setCurrent] = useState<number | null>(null);
  const people = usePeople(q);
  const candidates = useCandidates();
  const sync = useSyncCRM();
  const session = useSession();
  const projects = useProjects();
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";
  const projectName = (id: number | null) =>
    projects.data?.projects.find((p) => p.id === id)?.name ?? (id ? `project ${id}` : "no project");
  return (
    <section className="grid gap-6 text-sm md:grid-cols-[20rem_1fr]">
      <aside className="space-y-3">
        <div className="flex items-baseline justify-between gap-2">
          <h1 className="text-3xl font-semibold tracking-tight">People</h1>
          {canEdit && (
            <Button
              size="xs"
              variant="outline"
              disabled={sync.isPending}
              onClick={() => sync.mutate()}
            >
              Resolve new records
            </Button>
          )}
        </div>
        {sync.data && (
          <p className="text-muted-foreground">
            Linked {sync.data.linked} record{sync.data.linked === 1 ? "" : "s"}.
          </p>
        )}
        <Input
          placeholder="Search name, phone or email"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        {candidates.data && candidates.data.candidates.length > 0 && (
          <p className="rounded-md border border-destructive/40 p-2 text-destructive">
            {candidates.data.candidates.length} uncertain match
            {candidates.data.candidates.length === 1 ? "" : "es"} waiting for a decision.
          </p>
        )}
        {people.error && <p className="text-destructive">{people.error.message}</p>}
        <ul className="space-y-1">
          {people.data?.people.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${p.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(p.id)}
              >
                {p.displayName || "(no name)"}
                <span className="block text-muted-foreground">
                  {p.primaryPhone ?? ""} {p.primaryEmail ?? ""} · {p.enquiries} enquir
                  {p.enquiries === 1 ? "y" : "ies"}
                  {p.uncertain ? " · uncertain match" : ""}
                </span>
              </button>
            </li>
          ))}
          {people.data?.people.length === 0 && (
            <li className="text-muted-foreground">
              Nobody yet. Import leads or resolve new records.
            </li>
          )}
        </ul>
        {canEdit && <NewPerson onCreated={setCurrent} />}
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick a person.</p>
        ) : (
          <PersonView id={current} canEdit={canEdit} projectName={projectName} />
        )}
      </div>
    </section>
  );
}

function NewPerson({ onCreated }: { onCreated: (id: number) => void }) {
  const create = useCreatePerson();
  const [form, setForm] = useState({
    displayName: "",
    phone: "",
    email: "",
    country: "AE",
    language: "en",
    notes: "",
  });
  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(form, { onSuccess: (d) => onCreated(d.person.id) });
  }
  return (
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3">
      <h2 className="font-semibold">Add a person by hand</h2>
      <Input
        placeholder="Name"
        value={form.displayName}
        onChange={(e) => setForm({ ...form, displayName: e.target.value })}
      />
      <Input
        placeholder="Phone"
        value={form.phone}
        onChange={(e) => setForm({ ...form, phone: e.target.value })}
      />
      <Input
        placeholder="Email"
        value={form.email}
        onChange={(e) => setForm({ ...form, email: e.target.value })}
      />
      <Button type="submit" size="sm" disabled={create.isPending}>
        Add
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function PersonView({
  id,
  canEdit,
  projectName,
}: {
  id: number;
  canEdit: boolean;
  projectName: (id: number | null) => string;
}) {
  const q = usePerson(id);
  const decide = useDecideCandidate();
  const unmerge = useUnmergePerson();
  const enrich = useEnrichPerson();
  const setStated = useSetStated();
  const reverse = useReverseLink();
  const [attr, setAttr] = useState({ key: "", value: "" });
  const [brief, setBrief] = useState<number | null>(null);
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  if (q.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (q.error) return <p className="text-destructive">{q.error.message}</p>;
  const d = q.data;
  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">
          {d.person.displayName || "(no name)"}
        </h2>
        <p className="text-muted-foreground">
          {d.person.primaryPhone ?? "no phone"} · {d.person.primaryEmail ?? "no email"} · first seen{" "}
          {new Date(d.person.createdAt).toLocaleDateString()}
        </p>
        {d.person.mergedIntoId !== null && (
          <p className="text-destructive">
            Merged into person {d.person.mergedIntoId}.
            {canEdit && (
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => unmerge.mutate({ id, reason: "undone from the person page" })}
              >
                Undo merge
              </Button>
            )}
          </p>
        )}
      </div>

      {d.candidates.length > 0 && (
        <section className="space-y-2 rounded-lg border border-destructive/40 p-3">
          <h3 className="font-semibold">Uncertain matches</h3>
          <p className="text-muted-foreground">
            These records share something with this person but not enough to be sure. Nothing is
            merged until someone says so.
          </p>
          {d.candidates.map((c) => (
            <div key={c.id} className="flex flex-wrap items-center gap-2 border-t pt-2">
              <span>
                {c.other
                  ? `${c.other.displayName || "(no name)"} · ${c.other.primaryPhone ?? ""} ${c.other.primaryEmail ?? ""}`
                  : `person ${c.candidateId}`}
              </span>
              <span className="text-muted-foreground">why: {JSON.stringify(c.reason)}</span>
              {canEdit && (
                <>
                  <Button
                    size="xs"
                    onClick={() => decide.mutate({ id: c.id, accept: true, note: "same person" })}
                  >
                    Same person (merge)
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      decide.mutate({ id: c.id, accept: false, note: "different people" })
                    }
                  >
                    Different people
                  </Button>
                </>
              )}
            </div>
          ))}
          {decide.error && <p className="text-destructive">{decide.error.message}</p>}
        </section>
      )}

      <section className="space-y-1">
        <h3 className="font-semibold">Enquiries</h3>
        {d.enquiries.length === 0 && <p className="text-muted-foreground">None.</p>}
        {d.enquiries.map((e) => (
          <div key={e.id} className="space-y-1">
            <p>
              {projectName(e.projectId)} · {e.state}
              {e.stage ? ` · ${e.stage}` : ""} · from {e.sourceKind} {e.sourceRef} · first{" "}
              {new Date(e.firstContactAt).toLocaleDateString()} · last{" "}
              {new Date(e.lastActivityAt).toLocaleDateString()}
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => setBrief(brief === e.id ? null : e.id)}
              >
                {brief === e.id ? "Hide brief" : "Sales brief"}
              </Button>
            </p>
            {brief === e.id && <BriefPanel enquiryId={e.id} canEdit={canEdit} isAdmin={isAdmin} />}
          </div>
        ))}
      </section>

      <section className="space-y-1">
        <h3 className="font-semibold">Identifiers and sources</h3>
        {d.identifiers.map((i) => (
          <p key={i.id}>
            {i.kind}: {i.value}{" "}
            <span className="text-muted-foreground">
              · {i.class} · from {i.source}
              {i.evidenceId ? ` · evidence ${i.evidenceId}` : ""}
            </span>
          </p>
        ))}
        {d.links.map((l) => (
          <p key={l.id} className={l.reversedAt ? "text-muted-foreground line-through" : ""}>
            {l.kind} {l.ref} · matched as {l.matchLevel} · {JSON.stringify(l.matchEvidence)}
            {l.reversedAt && ` · reversed: ${l.reversalReason}`}
            {canEdit && !l.reversedAt && (
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => reverse.mutate({ id: l.id, reason: "wrong person" })}
              >
                Not this person
              </Button>
            )}
          </p>
        ))}
        {reverse.error && <p className="text-destructive">{reverse.error.message}</p>}
      </section>

      <ScoreSection id={id} canEdit={canEdit} />

      <section className="space-y-2">
        <h3 className="font-semibold">
          What we know{" "}
          <span className="font-normal text-muted-foreground">
            (what the customer said stands; a provider's guess sits beside it)
          </span>
        </h3>
        {d.attributes.length === 0 && <p className="text-muted-foreground">Nothing recorded.</p>}
        {d.attributes.map((a) => (
          <p key={a.key} className={a.conflict ? "text-destructive" : ""}>
            {a.key}: {a.value} <span className="text-muted-foreground">({a.class})</span>
            {a.enriched && a.class === "stated" && (
              <span className="text-muted-foreground">
                {" "}
                · provider says "{a.enriched.value}" ({a.enriched.source},{" "}
                {new Date(a.enriched.observedAt).toLocaleDateString()})
                {a.conflict ? " — disagrees" : ""}
              </span>
            )}
          </p>
        ))}
        {canEdit && (
          <div className="flex flex-wrap items-center gap-1">
            <Input
              className="h-7 max-w-32"
              placeholder="key"
              value={attr.key}
              onChange={(e) => setAttr({ ...attr, key: e.target.value })}
            />
            <Input
              className="h-7 max-w-48"
              placeholder="what the customer said"
              value={attr.value}
              onChange={(e) => setAttr({ ...attr, value: e.target.value })}
            />
            <Button
              size="xs"
              variant="outline"
              disabled={!attr.key || !attr.value || setStated.isPending}
              onClick={() =>
                setStated.mutate({ id, key: attr.key, value: attr.value, source: "manual" })
              }
            >
              Record as stated
            </Button>
            {d.enrichment.configured ? (
              <Button
                size="xs"
                variant="outline"
                disabled={enrich.isPending}
                onClick={() => enrich.mutate(id)}
              >
                Enrich via {d.enrichment.provider} ({d.enrichment.currency}{" "}
                {(d.enrichment.costMinor / 100).toFixed(2)})
              </Button>
            ) : (
              <span className="text-muted-foreground">Enrichment: {d.enrichment.reason}</span>
            )}
          </div>
        )}
        {setStated.error && <p className="text-destructive">{setStated.error.message}</p>}
        {enrich.error && <p className="text-destructive">{enrich.error.message}</p>}
        <p className="text-muted-foreground">
          Permitted attributes: {d.enrichment.permitted.join(", ")}.
        </p>
      </section>
    </div>
  );
}
