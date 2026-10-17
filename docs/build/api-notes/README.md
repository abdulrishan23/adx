# Provider API notes

Facts read from each provider's current primary documentation on 2026-10-17, with URLs, by research agents during the build. The adapters in `server/providers/` are written from these notes, not from memory. Anything the docs did not confirm is marked "unconfirmed" and is checked against a live response on first use (recorded in `../verification.md`).

- `google-ads.md`: Google Ads API v26 REST (searchStream, GAQL fields, hierarchy, conversion actions, errors, quotas, freshness).
- `meta-marketing.md`: Meta Marketing API / Graph v26.0 (ad accounts, insights, paging, objects, lead ads, errors, rate headers, freshness).
- `gohighlevel.md`: HighLevel API v2 (2021-07-28) contact search, custom fields, opportunities, webhooks, limits, scopes.
