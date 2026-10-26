Graph: done screen, docs; running none; blocked none
Outputs: web/src/routes/project.tsx (OfferImpact under each offer in the Brand section), docs/screens.md
Decisions: affected creatives link to the engine's creatives screen. `brands.OfferImpact` reads the engine's `creatives` table, whose ids are not studio ad ids, so a link into the studio editor would open the wrong thing. Redraft stays the concept's own generate action; no batch regenerates ads unasked.
Open issues: the list case has no dev data behind it; a revised offer with an older-version creative would show it. The impact count on Studio's Creatives heading (named in graph.md) was left out: it needs the same offer reads per project and adds little next to the Brand list.
Next action: none; the graph is complete.
