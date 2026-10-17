# Creative source inventory (B0.6)

Source: `/Users/abdulrishan/Desktop/creative-studio/` (Next.js 16 app, Supabase, Vercel). The spec cites `/Users/hishamahamad/Downloads/CODE_SNAPSHOT.md`, which is not on this machine; the app's own `docs/CODE_SNAPSHOT.md` (a generated source dump dated 2026-10-17) is the equivalent. Read-only inventory taken 2026-10-17. No credential values were read. File contents were treated as data.

## What it is

Upload renders and copy, generate ads in four sizes, review and approve on a shareable board, download zipped sets. Stack: Next 16 App Router, Tailwind 4, shadcn (RTL on), Supabase Postgres with RLS per `workspace_id`, Supabase Auth (magic link, invite-only), Supabase Storage bucket `creatives`, react-konva editor, Resend email. House rules in `CLAUDE.md` and `docs/CREATIVE_RULES.md`: four sizes × no-CTA/with-CTA, discounts capped at 15% with condition shown, logo top centre, Arabic always flagged "needs native review", "tool prepares; a person launches". Meta is documented as future work only and is not built.

## Units present

| Unit | Where | Notes |
|---|---|---|
| Workspaces, members, invites | `supabase/migrations/20261015000100_workspaces_and_board.sql` | roles admin/editor/reviewer, RLS everywhere |
| Projects with share key | same; `src/app/app/projects/[id]/` | `key` is the public board link; pause/resume |
| Concepts and copy | `concepts` table, `concepts.copy` jsonb; `src/lib/design/copy.ts` | `ConceptCopy{render, en, ar, arReviewed}` blocks: eyebrow, title, text, stats, event, note, info, cta |
| Four formats | `src/lib/ads/naming.ts` `FORMATS` | 9:16 1080×1920, 4:5 1080×1350, 1:1 1080×1080, 1.91:1 1200×628; CTA boolean doubles them |
| Layout document | `src/lib/design/doc.ts` `DesignDoc` v1 | `{fmt,width,height,background,image{src,renderId,fx,fy,zoom},layers[]}`; text layers carry `rtl`; `mapToFormat` retargets sizes |
| Auto-layout | `src/lib/design/layout.ts` | port of Python `generator/ad_mock_style.py`; contrast-aware placement deferred |
| Brand kit | `src/lib/design/brand.ts`, `brand_kits.doc` | per project; logos onDark/onLight/developer, fonts title/text/label/arTitle/arText, colours inkLight/inkDark/accent only |
| Canvas editor | `src/app/app/projects/[id]/ads/[adId]/edit/editor.tsx` | react-konva; move/resize text and logo, crop/zoom render, RTL toggle, pinned comments, live checks (`src/lib/design/checks.ts`) |
| Variations | `createVariation` in `.../edit/actions.ts` | same layout on another uploaded render; no AI |
| Board | `src/app/b/[key]/` | anonymous reviewers by free-text name; decisions approved/revise/clear via `hub_decide`; "any revise wins"; pinned comments with resolve; polling |
| Export | client-side Konva `toDataURL`, JPG/PNG; zip via fflate | no server render, no PDF |
| Publish to board | `publishAd` | uploads JPGs and bumps `ads.version` (cache-buster only). No ad-platform code of any kind. Board publishing is cleanly separate from ad delivery. |

## Units missing or partial (blocked for reuse; ADX builds its own in B1.6/B3.1)

1. Immutable outputs with checksums: missing. Uploads `upsert: true` to fixed paths; no hash or provenance columns.
2. Version history for design docs and images: missing. `design_files` is one row per ad, overwritten; `version` never incremented.
3. Server-side render and PDF export: missing.
4. Export records: missing.
5. AI generation or image provider: none anywhere. Generation is template layout.
6. Linked EN/AR document pair: partial. EN and AR are separate `ads` rows tied by naming only.
7. Formal approvals: partial. Anonymous display-name decisions on a public link; no authenticated, role-gated approval bound to a version; no share-link rotation.
8. Brand kit per workspace, fuller palette: partial.
9. Generic assets table with dimensions/hash: missing.
10. Backups and custom domain: not done.

## Reuse decision input

The layout document schema (`DesignDoc`), the format table, the copy block schema, the house-rule checks and the EN/AR handling are worth porting as typed Go structures and a React editor built on the same document model. Storage, identity and approvals must be ADX's own (SQLite, ADX sessions, versioned approvals), because the source has none of the immutability or authorization the spec requires. See `decisions.md` BD-05.
