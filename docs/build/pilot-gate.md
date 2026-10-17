# Phase 1 pilot gate (B1.6)

Taken on 2026-10-17 at the end of phase 1. The local journey is implemented and tested; the connected pilot needs inputs that don't exist on this machine. Nothing here claims verification it didn't do.

## The journey, locally

| Step | State | Evidence |
|---|---|---|
| Sign in with a verified session, scoped to the Reportage workspace | implemented locally | `core/identity`, `api/auth_test.go` |
| Connect GoHighLevel (private token) and import leads | implemented locally | `core/leads`, `providers/gohighlevel/contacts_test.go` |
| Connect Google Ads and Meta by OAuth, discover accounts, refresh campaigns and daily figures | implemented locally | `core/reporting/ingest_test.go`, `providers/googleads`, `providers/meta`, `core/connections` ads test |
| Deterministic analytics with coverage, freshness and comparisons | implemented locally | `core/reporting/metrics_test.go` |
| Scheduled reports and anomaly alerts, unconfigured by default | implemented locally | `core/reports`, `core/alerts` |
| Ask ADX, read-only, grounded | implemented locally | `core/assistant`, `providers/anthropic` |
| Upload a render, lay out copy in 9:16 / 4:5 / 1:1 / 1.91:1 in English or Arabic, render, review board, approve, export | implemented locally | `core/creative`, `core/assets/layout_test.go`, `providers/svgrender` |

## What the connected pilot still needs

| Input | Who | Blocks |
|---|---|---|
| authentik `adx-auth` provider client id/secret, redirect URI, groups scope mapping | WokeWorks (D-24) | live sign-in |
| GoHighLevel private integration token for the ADX Test location, saved with `ADX_CONNECTION_KEY_FILE` | Reportage GHL admin / Abdul | live lead import; confirms three unconfirmed API details (BD-20) |
| Which GHL field holds the lead status, and the status-to-stage table | Abdul | stage classification, qualified leads, deals |
| Google Cloud OAuth client on a project with Google Ads API access; the Head of Marketing & AI's sign-in | Reportage / WokeWorks | live Google reads, same-day reconciliation against the Google Ads UI |
| Meta Business app id/secret; the connecting person given a role on the app (D-22) | WokeWorks / Reportage | live Meta reads and reconciliation |
| Hosting approval in writing naming the server location (D-21, #35) | Reportage | moving any real lead data |
| SMTP host and a shared sender (D-24) and report recipients | WokeWorks / Abdul | report delivery |
| Anomaly thresholds | Abdul / CMO | alerts firing |
| Model API key and data-use approval | Abdul | live Ask ADX answers |

## Not done in phase 1

- No browser session was run against the dev server in this build; the web pages pass typecheck and lint but were not inspected visually. In particular, long Arabic copy in the four formats has not been inspected visually (acceptance item of B1.6): the layout and the RTL rendering are covered by unit tests on the document and the SVG, which is not the same thing.
- The canvas drag editor from the creative source is not ported; the pilot editor changes text and vertical position only (B3.1).
- Pixel export (PNG/JPG) is not implemented; the output is SVG, rasterized by the browser when viewed. A server-side rasterizer or a browser export is B3.1.
- Live calling, messaging and any ad-account write are not part of phase 1 by design.
