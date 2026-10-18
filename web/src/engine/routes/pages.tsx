import { type FormEvent, useState } from "react";
import type { FormField, PageDetail, PageDocument, Section } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useBrand, useBrands } from "@/engine/api/brands";
import { useProjectOffers } from "@/engine/api/creative";
import {
  useCreatePage,
  usePage,
  usePagePreview,
  usePageReadiness,
  usePageSubmissions,
  usePages,
  usePublishPage,
  useRollbackPage,
  useSavePageVersion,
  useTestSubmission,
} from "@/engine/api/pages";
import { useSession } from "@/engine/api/session";
import { StatusChip, type StatusTone } from "@/engine/components/status";

const emptyDoc: PageDocument = {
  title: "",
  headline: "",
  subheadline: "",
  sections: [
    { kind: "text", text: "", items: [], imageKey: "", alt: "" },
    { kind: "form", text: "", items: [], imageKey: "", alt: "" },
  ],
  offerId: null,
  offerVersion: null,
  form: {
    fields: [
      { name: "name", label: "Name", kind: "text", required: true, options: [] },
      { name: "phone", label: "Phone", kind: "phone", required: true, options: [] },
      { name: "email", label: "Email", kind: "email", required: false, options: [] },
    ],
    consentText: "",
    submitLabel: "Register",
    successMessage: "Thank you. We will be in touch.",
  },
  footer: "",
};

// Landing pages: structured, versioned, server-rendered with no script.
// Publish makes a version live at /p/{slug}; rollback republishes the one
// before and names it. Readiness needs a live page, a form that can reach
// someone, a test submission on that very version, and approved claims.
export function PagesPage() {
  const pages = usePages();
  const session = useSession();
  const [current, setCurrent] = useState<number | null>(null);
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";
  return (
    <section className="grid gap-6 md:grid-cols-[16rem_1fr]">
      <aside className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">Pages</h1>
        {pages.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {pages.error && <p className="text-sm text-destructive">{pages.error.message}</p>}
        {pages.data?.pages.length === 0 && (
          <p className="text-sm text-muted-foreground">No pages yet.</p>
        )}
        <ul className="space-y-1 text-sm">
          {pages.data?.pages.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${p.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(p.id)}
              >
                {p.name}{" "}
                <span className="text-muted-foreground">
                  · /p/{p.slug} · {p.language}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {canEdit && <NewPage onCreated={setCurrent} />}
        {pages.data && (
          <div className="rounded-lg border p-3 text-sm">
            <h2 className="font-semibold">Publishing</h2>
            <p className="text-muted-foreground">
              Pages are served by ADX ({pages.data.publishing.local}).
            </p>
            {!pages.data.publishing.external.available && (
              <p className="text-destructive">
                External hosts: blocked. {pages.data.publishing.external.reason}. Unblocked by:{" "}
                {pages.data.publishing.external.unblocks}.
              </p>
            )}
          </div>
        )}
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick or create a page.</p>
        ) : (
          <PageView id={current} canEdit={canEdit} />
        )}
      </div>
    </section>
  );
}

function NewPage({ onCreated }: { onCreated: (id: number) => void }) {
  const brands = useBrands();
  const create = useCreatePage();
  const [brandId, setBrandId] = useState(0);
  const brand = useBrand(brandId || null);
  const [form, setForm] = useState({
    projectId: 0,
    slug: "",
    name: "",
    language: "en",
    title: "",
    headline: "",
  });
  const own = brands.data?.brands.filter((b) => b.kind === "own") ?? [];
  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      {
        brandId,
        projectId: form.projectId,
        planId: 0,
        planVersion: 0,
        slug: form.slug,
        name: form.name,
        language: form.language,
        document: { ...emptyDoc, title: form.title, headline: form.headline },
      },
      { onSuccess: (d) => onCreated(d.page.id) },
    );
  }
  return (
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">New page</h2>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={brandId}
        onChange={(e) => {
          setBrandId(Number(e.target.value));
          setForm({ ...form, projectId: 0 });
        }}
      >
        <option value={0}>Brand…</option>
        {own.map((b) => (
          <option key={b.id} value={b.id}>
            {b.name}
          </option>
        ))}
      </select>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.projectId}
        onChange={(e) => setForm({ ...form, projectId: Number(e.target.value) })}
        disabled={!brandId}
      >
        <option value={0}>Project…</option>
        {brand.data?.projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <Input
        placeholder="Name (internal)"
        value={form.name}
        onChange={(e) => setForm({ ...form, name: e.target.value })}
      />
      <Input
        placeholder="slug, e.g. wadi-launch"
        value={form.slug}
        onChange={(e) => setForm({ ...form, slug: e.target.value })}
      />
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.language}
        onChange={(e) => setForm({ ...form, language: e.target.value })}
      >
        <option value="en">English</option>
        <option value="ar">Arabic</option>
      </select>
      <Input
        placeholder="Page title"
        value={form.title}
        onChange={(e) => setForm({ ...form, title: e.target.value })}
        dir="auto"
      />
      <Input
        placeholder="Headline"
        value={form.headline}
        onChange={(e) => setForm({ ...form, headline: e.target.value })}
        dir="auto"
      />
      <Button type="submit" size="sm" disabled={create.isPending || !brandId || !form.projectId}>
        Create (version 1)
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function PageView({ id, canEdit }: { id: number; canEdit: boolean }) {
  const page = usePage(id);
  const [version, setVersion] = useState<number | null>(null);
  if (page.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (page.error) return <p className="text-destructive">{page.error.message}</p>;
  const d = page.data;
  const latest = d.versions[d.versions.length - 1];
  const shown = d.versions.find((v) => v.version === version) ?? latest;
  if (!shown) return null;
  return (
    <div className="space-y-4 text-sm">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">{d.page.name}</h2>
          <p className="text-muted-foreground">
            <a href={d.publicUrl} target="_blank" rel="noreferrer noopener" className="underline">
              {d.publicUrl}
            </a>
            {d.current ? ` · live: version ${d.current.version}` : " · not published"}
          </p>
        </div>
        <div className="flex flex-wrap gap-1">
          {d.versions.map((v) => (
            <Button
              key={v.id}
              size="xs"
              variant={v.version === shown.version ? "default" : "outline"}
              onClick={() => setVersion(v.version)}
            >
              v{v.version}
              {d.current?.version === v.version ? " ● live" : ""}
            </Button>
          ))}
        </div>
      </div>
      <ReadinessPanel detail={d} canEdit={canEdit} />
      <div className="grid gap-4 md:grid-cols-2">
        <Editor
          key={shown.id}
          detail={d}
          version={shown.version}
          doc={shown.document}
          canEdit={canEdit}
        />
        <Preview pageId={id} version={shown.version} />
      </div>
      <Deployments detail={d} canEdit={canEdit} />
      <Submissions pageId={id} />
    </div>
  );
}

function ReadinessPanel({ detail, canEdit }: { detail: PageDetail; canEdit: boolean }) {
  const readiness = usePageReadiness(detail.page.id);
  const test = useTestSubmission();
  const r = readiness.data;
  const live = detail.current;
  const form = detail.versions.find((v) => v.version === live?.version)?.document.form;
  const sample: Record<string, string> = {};
  for (const f of form?.fields ?? []) {
    if (f.kind === "email") sample[f.name] = "test@reportage.example";
    else if (f.kind === "phone") sample[f.name] = "+971500000000";
    else if (f.kind === "select") sample[f.name] = f.options[0] ?? "";
    else sample[f.name] = "ADX test";
  }
  return (
    <section className="rounded-lg border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-semibold">
          {r ? (r.ready ? "Ready for launch review" : "Not ready for launch") : "Readiness"}
        </h3>
        {canEdit && live && detail.testToken && (
          <Button
            size="xs"
            variant="outline"
            disabled={test.isPending}
            onClick={() =>
              test.mutate({
                slug: detail.page.slug,
                token: detail.testToken,
                project: detail.page.projectId ?? 0,
                fields: sample,
              })
            }
          >
            Send test submission to live version
          </Button>
        )}
      </div>
      <p className="text-muted-foreground">
        Ready means live, a form that reaches someone and was tested on this version, approved
        claims. It is not a launch.
      </p>
      {test.error && <p className="text-destructive">Test failed: {test.error.message}</p>}
      {test.data && (
        <p className="text-muted-foreground">Test submission recorded and marked as a test.</p>
      )}
      {readiness.error && <p className="text-destructive">{readiness.error.message}</p>}
      {r && (
        <ul className="mt-1 space-y-1">
          {r.blockers.map((b) => (
            <li key={`${b.code}:${b.message}`} className="text-destructive">
              Blocks: {b.message}
            </li>
          ))}
          {r.warnings.map((w) => (
            <li key={`${w.code}:${w.message}`} className="text-muted-foreground">
              Note: {w.message}
            </li>
          ))}
          {r.claims.map((c) => (
            <li key={c.text} className={c.status === "approved" ? "" : "text-destructive"}>
              Claim "{c.text}": {c.status} ({c.detail})
            </li>
          ))}
          {r.lastTest && (
            <li className="text-muted-foreground">
              Last test on live version: {r.lastTest.state}
              {r.lastTest.reason ? ` (${r.lastTest.reason})` : ""} at{" "}
              {new Date(r.lastTest.receivedAt).toLocaleString()}
            </li>
          )}
        </ul>
      )}
    </section>
  );
}

function Preview({ pageId, version }: { pageId: number; version: number }) {
  const preview = usePagePreview(pageId, version);
  return (
    <div className="space-y-1">
      <p className="font-medium">
        Preview v{version}{" "}
        <span className="font-normal text-muted-foreground">(sandboxed; no script can run)</span>
      </p>
      {preview.error && <p className="text-destructive">{preview.error.message}</p>}
      <iframe
        title={`Preview of version ${version}`}
        sandbox=""
        srcDoc={preview.data ?? ""}
        className="h-[36rem] w-full rounded-lg border bg-black"
      />
    </div>
  );
}

let nextRowId = 1;
const rowId = () => `row-${nextRowId++}`;

function Editor({
  detail,
  version,
  doc: initial,
  canEdit,
}: {
  detail: PageDetail;
  version: number;
  doc: PageDocument;
  canEdit: boolean;
}) {
  const [doc, setDoc] = useState<PageDocument>(initial);
  const [dirty, setDirty] = useState(false);
  // Rows carry client-side ids so React keys survive removal and reordering.
  const [sectionIds, setSectionIds] = useState<string[]>(() => initial.sections.map(rowId));
  const [fieldIds, setFieldIds] = useState<string[]>(() => (initial.form?.fields ?? []).map(rowId));
  const save = useSavePageVersion(detail.page.id);
  const offers = useProjectOffers(detail.page.projectId ?? 0);
  const update = (next: PageDocument) => {
    setDoc(next);
    setDirty(true);
  };
  const sections = doc.sections;
  const setSection = (i: number, s: Section) =>
    update({ ...doc, sections: sections.map((x, j) => (j === i ? s : x)) });
  const form = doc.form;
  const setField = (i: number, f: FormField) =>
    form &&
    update({ ...doc, form: { ...form, fields: form.fields.map((x, j) => (j === i ? f : x)) } });
  const kinds = ["heading", "text", "bullets", "image", "offer", "form"];
  return (
    <div className="space-y-2">
      <p className="font-medium">Document v{version}</p>
      <Input
        placeholder="Title"
        value={doc.title}
        onChange={(e) => update({ ...doc, title: e.target.value })}
        disabled={!canEdit}
        dir="auto"
      />
      <Input
        placeholder="Headline"
        value={doc.headline}
        onChange={(e) => update({ ...doc, headline: e.target.value })}
        disabled={!canEdit}
        dir="auto"
      />
      <Input
        placeholder="Subheadline"
        value={doc.subheadline}
        onChange={(e) => update({ ...doc, subheadline: e.target.value })}
        disabled={!canEdit}
        dir="auto"
      />
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={doc.offerId ?? 0}
        onChange={(e) =>
          update({ ...doc, offerId: Number(e.target.value) || null, offerVersion: null })
        }
        disabled={!canEdit}
      >
        <option value={0}>No offer on this page</option>
        {offers.data?.offers.map((o) => (
          <option key={o.id} value={o.id}>
            {o.name} (v{o.current.version}, {o.current.state})
          </option>
        ))}
      </select>
      <div className="space-y-1">
        <p className="text-muted-foreground">Sections</p>
        {sections.map((s, i) => (
          <div
            key={sectionIds[i] ?? `${s.kind}-${s.text}`}
            className="flex flex-wrap items-center gap-1 rounded-md border p-1"
          >
            <select
              className="rounded-md border bg-background px-1"
              value={s.kind}
              onChange={(e) => setSection(i, { ...s, kind: e.target.value })}
              disabled={!canEdit}
            >
              {kinds.map((k) => (
                <option key={k}>{k}</option>
              ))}
            </select>
            {(s.kind === "heading" || s.kind === "text") && (
              <Input
                className="h-7 flex-1"
                placeholder="Text"
                value={s.text}
                onChange={(e) => setSection(i, { ...s, text: e.target.value })}
                disabled={!canEdit}
                dir="auto"
              />
            )}
            {s.kind === "bullets" && (
              <Input
                className="h-7 flex-1"
                placeholder="Items separated by ;"
                value={s.items.join("; ")}
                onChange={(e) =>
                  setSection(i, { ...s, items: e.target.value.split(";").map((x) => x.trim()) })
                }
                disabled={!canEdit}
                dir="auto"
              />
            )}
            {s.kind === "image" && (
              <>
                <Input
                  className="h-7 flex-1"
                  placeholder="Asset key (sha256-…)"
                  value={s.imageKey}
                  onChange={(e) => setSection(i, { ...s, imageKey: e.target.value })}
                  disabled={!canEdit}
                />
                <Input
                  className="h-7 max-w-40"
                  placeholder="Alt text"
                  value={s.alt}
                  onChange={(e) => setSection(i, { ...s, alt: e.target.value })}
                  disabled={!canEdit}
                />
              </>
            )}
            {s.kind === "offer" && (
              <span className="text-muted-foreground">shows the bound offer's approved terms</span>
            )}
            {s.kind === "form" && <span className="text-muted-foreground">places the form</span>}
            {canEdit && (
              <Button
                size="xs"
                variant="outline"
                onClick={() => {
                  setSectionIds(sectionIds.filter((_, j) => j !== i));
                  update({ ...doc, sections: sections.filter((_, j) => j !== i) });
                }}
              >
                ×
              </Button>
            )}
          </div>
        ))}
        {canEdit && (
          <Button
            size="xs"
            variant="outline"
            onClick={() => {
              setSectionIds([...sectionIds, rowId()]);
              update({
                ...doc,
                sections: [
                  ...sections,
                  { kind: "text", text: "", items: [], imageKey: "", alt: "" },
                ],
              });
            }}
          >
            Add section
          </Button>
        )}
      </div>
      <div className="space-y-1 rounded-md border p-2">
        <p className="text-muted-foreground">Form</p>
        {form ? (
          <>
            {form.fields.map((f, i) => (
              <div key={fieldIds[i] ?? f.name} className="flex flex-wrap items-center gap-1">
                <Input
                  className="h-7 max-w-28"
                  placeholder="name"
                  value={f.name}
                  onChange={(e) => setField(i, { ...f, name: e.target.value })}
                  disabled={!canEdit}
                />
                <Input
                  className="h-7 max-w-32"
                  placeholder="Label"
                  value={f.label}
                  onChange={(e) => setField(i, { ...f, label: e.target.value })}
                  disabled={!canEdit}
                />
                <select
                  className="rounded-md border bg-background px-1"
                  value={f.kind}
                  onChange={(e) => setField(i, { ...f, kind: e.target.value })}
                  disabled={!canEdit}
                >
                  {["text", "email", "phone", "select"].map((k) => (
                    <option key={k}>{k}</option>
                  ))}
                </select>
                <label className="flex items-center gap-1 text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={f.required}
                    onChange={(e) => setField(i, { ...f, required: e.target.checked })}
                    disabled={!canEdit}
                  />
                  required
                </label>
                {f.kind === "select" && (
                  <Input
                    className="h-7 max-w-40"
                    placeholder="options; separated"
                    value={f.options.join("; ")}
                    onChange={(e) =>
                      setField(i, {
                        ...f,
                        options: e.target.value
                          .split(";")
                          .map((o) => o.trim())
                          .filter(Boolean),
                      })
                    }
                    disabled={!canEdit}
                  />
                )}
                {canEdit && (
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() => {
                      setFieldIds(fieldIds.filter((_, j) => j !== i));
                      update({
                        ...doc,
                        form: { ...form, fields: form.fields.filter((_, j) => j !== i) },
                      });
                    }}
                  >
                    ×
                  </Button>
                )}
              </div>
            ))}
            {canEdit && (
              <Button
                size="xs"
                variant="outline"
                onClick={() => {
                  setFieldIds([...fieldIds, rowId()]);
                  update({
                    ...doc,
                    form: {
                      ...form,
                      fields: [
                        ...form.fields,
                        { name: "", label: "", kind: "text", required: false, options: [] },
                      ],
                    },
                  });
                }}
              >
                Add field
              </Button>
            )}
            <Input
              placeholder="Consent text"
              value={form.consentText}
              onChange={(e) => update({ ...doc, form: { ...form, consentText: e.target.value } })}
              disabled={!canEdit}
              dir="auto"
            />
            <Input
              placeholder="Submit button"
              value={form.submitLabel}
              onChange={(e) => update({ ...doc, form: { ...form, submitLabel: e.target.value } })}
              disabled={!canEdit}
              dir="auto"
            />
            <Input
              placeholder="Success message"
              value={form.successMessage}
              onChange={(e) =>
                update({ ...doc, form: { ...form, successMessage: e.target.value } })
              }
              disabled={!canEdit}
              dir="auto"
            />
          </>
        ) : (
          canEdit && (
            <Button
              size="xs"
              variant="outline"
              onClick={() => {
                setFieldIds((emptyDoc.form?.fields ?? []).map(rowId));
                update({ ...doc, form: emptyDoc.form });
              }}
            >
              Add a form
            </Button>
          )
        )}
      </div>
      <Input
        placeholder="Footer"
        value={doc.footer}
        onChange={(e) => update({ ...doc, footer: e.target.value })}
        disabled={!canEdit}
        dir="auto"
      />
      {canEdit && (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            disabled={!dirty || save.isPending}
            onClick={() =>
              save.mutate({ document: doc, note: "" }, { onSuccess: () => setDirty(false) })
            }
          >
            Save as version {detail.versions.length + 1}
          </Button>
          {save.error && <span className="text-destructive">{save.error.message}</span>}
        </div>
      )}
    </div>
  );
}

function Deployments({ detail, canEdit }: { detail: PageDetail; canEdit: boolean }) {
  const publish = usePublishPage(detail.page.id);
  const rollback = useRollbackPage(detail.page.id);
  const latest = detail.versions[detail.versions.length - 1];
  return (
    <section className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-semibold">Deployments</h3>
        {canEdit && latest && (
          <Button
            size="xs"
            disabled={publish.isPending || detail.current?.version === latest.version}
            onClick={() => publish.mutate(latest.version)}
          >
            Publish version {latest.version}
          </Button>
        )}
        {canEdit && detail.current && (
          <Button
            size="xs"
            variant="outline"
            disabled={rollback.isPending}
            onClick={() => rollback.mutate()}
          >
            Roll back to the previous version
          </Button>
        )}
        {publish.error && <span className="text-destructive">{publish.error.message}</span>}
        {rollback.error && <span className="text-destructive">{rollback.error.message}</span>}
        {rollback.data && (
          <span className="text-muted-foreground">
            Rolled back: version {rollback.data.version} is live again (from version{" "}
            {rollback.data.rolledBackFromVersion}).
          </span>
        )}
      </div>
      {detail.deployments.length === 0 && <p className="text-muted-foreground">Never published.</p>}
      <ul className="space-y-1">
        {detail.deployments.map((dp) => (
          <li
            key={dp.id}
            className={`flex flex-wrap items-center gap-x-2 gap-y-1 ${dp.state === "published" ? "" : "text-muted-foreground"}`}
          >
            <StatusChip tone={deploymentTone(dp.state)}>{dp.state}</StatusChip>v{dp.version} ·{" "}
            {dp.adapter} · {new Date(dp.deployedAt).toLocaleString()} ·{" "}
            {dp.outputSha256.slice(0, 12)}…
            {dp.rolledBackFromVersion !== null &&
              ` · rolled back from v${dp.rolledBackFromVersion}`}
            {dp.error && ` · ${dp.error}`}
          </li>
        ))}
      </ul>
    </section>
  );
}

// Published is live, failed is broken, anything else (an older deployment
// a newer one replaced) waits in outline.
function deploymentTone(state: string): StatusTone {
  if (state === "published") return "ready";
  if (state === "failed") return "error";
  return "pending";
}

function Submissions({ pageId }: { pageId: number }) {
  const subs = usePageSubmissions(pageId);
  return (
    <section className="space-y-2">
      <h3 className="font-semibold">
        Submissions{" "}
        <span className="font-normal text-muted-foreground">
          (test sends are marked and never count as enquiries)
        </span>
      </h3>
      {subs.data?.submissions.length === 0 && <p className="text-muted-foreground">None yet.</p>}
      <ul className="space-y-1">
        {subs.data?.submissions.map((s) => (
          <li key={s.id} className={s.state === "rejected" ? "text-muted-foreground" : ""}>
            {new Date(s.receivedAt).toLocaleString()} · {s.state}
            {s.reason ? ` (${s.reason})` : ""}
            {s.test ? " · TEST" : ""} ·{" "}
            {Object.entries(s.fields ?? {})
              .map(([k, v]) => `${k}: ${v}`)
              .join(", ")}
            {Object.keys(s.attribution ?? {}).length > 0 && (
              <span className="text-muted-foreground">
                {" "}
                ·{" "}
                {Object.entries(s.attribution ?? {})
                  .map(([k, v]) => `${k}=${v}`)
                  .join(" ")}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
