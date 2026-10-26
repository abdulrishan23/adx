| node | requirement | verdict | evidence |
|---|---|---|---|
| screen | under an offer, the creatives built on an older version are listed with review state and a link; nothing when none | pass (none case), unresolved (list case) | `OfferImpact` in web/src/routes/project.tsx reads `useOffer(id).affected`; dev app on 2026-10-26 shows nothing under "Launch offer", which has no affected creative. No revised offer in the dev data to show the list |
| screen | no batch redraft; the link goes to the creative | pass, with a deviation | no batch action exists. `AffectedCreative.id` is an engine creative id (`core/creative`), not a studio ad id, so the link opens `/engine/creatives`, where those creatives live, not the studio editor |
| docs | screens.md says so | pass | docs/screens.md, Projects rules |
