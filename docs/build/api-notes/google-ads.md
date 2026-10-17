# Google Ads API REST facts for a read-only Go client (checked 2026-10-17)

All facts below were read from the live pages on developers.google.com (fetched today). Items marked **unconfirmed** could not be found in the docs.

## 1. Versions, sunset, base URL

Source: https://developers.google.com/google-ads/api/docs/sunset-dates and https://developers.google.com/google-ads/api/docs/concepts/versioning

Timetable (released versions; v22 is no longer listed, so it has already sunset):

| Version | Release | Sunset |
|---|---|---|
| v23 / v23.1 / v23.2 | Jan 28 / Feb 25 / Mar 25, 2026 | February 2027 |
| v24 / v24.1 / v24.2 | Apr 22 / May 13 / Jun 24, 2026 | May / May / June 2027 |
| v25 / v25.1 / v25.2 | Jul 22 / Aug 19 / Sep 23, 2026 | August 2027 |
| **v26** | **October 14, 2026** | **November 2027** |
| v26.1, v26.2, v26.3* (upcoming) | Oct, Nov, Dec 2026 | November 2027 |
| v27 (upcoming) | January 2027 | February 2028 |

"We strive to keep at most five major versions available at any one time." "Major versions have a lifespan of around 12 months—minor versions: 10 months." "The tentative date means that the sunset could happen any time in that month." A new major version every 3 to 4 months.

Target **v26** (latest, released 3 days ago; sunset Nov 2027). Minor versions update the major endpoint in place: "Each major version has a separate REST and gRPC endpoint path (vX), which does not include the minor version suffix … https://googleads.googleapis.com/vX". Reference pages exist for v24, v25, v26.

Base URL: `https://googleads.googleapis.com/v26/customers/CUSTOMER_ID/googleAds` (https://developers.google.com/google-ads/api/rest/common/search). Discovery doc: `https://googleads.googleapis.com/$discovery/rest?version=v26` (https://developers.google.com/google-ads/api/rest/design/overview).

## 2. Authentication and headers

Sources: https://developers.google.com/google-ads/api/rest/auth , https://developers.google.com/google-ads/api/docs/concepts/call-structure , https://developers.google.com/google-ads/api/docs/first-call/dev-token

- Scope: `https://www.googleapis.com/auth/adwords` (user flow and service account flow). Token refresh endpoint shown: `POST https://www.googleapis.com/oauth2/v3/token` with `grant_type=refresh_token`.
- `Authorization: Bearer ACCESS_TOKEN` on every call; "An access token is valid for an hour after you acquire it".
- **Developer tokens are sunset.** dev-token page: "Developer tokens were sunset on September 9, 2026. … You can continue sending developer tokens in your API call headers, but this is optional and ignored by the API servers. … Your API access levels are determined by the Google Cloud project you used to generate your OAuth credentials." (user flow: the project owning the OAuth client ID/secret; service-account flow: the project owning the service account). "We will start rejecting developer tokens in API calls in a future major version." Access is applied for on the "Google Ads API Overview page in Google Cloud Console", not the Ads API Center. Note: the rest/auth page summary still says "A developer token is required in the developer-token HTTP header for every API call"; this is stale relative to the dev-token page. Recommendation: do not send `developer-token`.
- Enum reference confirms: `AuthorizationError.DEVELOPER_TOKEN_NOT_APPROVED` is "Deprecated: Developer tokens have been sunset. Superseded by CLOUD_PROJECT_NOT_APPROVED_FOR_PRODUCTION"; `DEVELOPER_TOKEN_PROHIBITED`, `DEVELOPER_TOKEN_NOT_ON_ALLOWLIST`, `AuthenticationError.DEVELOPER_TOKEN_INVALID` likewise deprecated (https://developers.google.com/google-ads/api/reference/rpc/v26/AuthorizationErrorEnum.AuthorizationError).
- `login-customer-id`: "This is the customer ID of the authorized customer to use in the request, without hyphens (-). If your access to the customer account is through a manager account, this header is required and must be set to the customer ID of the manager account. If you fail to include login-customer-id when you authenticate through a manager account, this results in an AuthorizationError.USER_PERMISSION_DENIED error." "If you don't include this header, it defaults to the operating customer" (the customer ID in the URL path). Not needed when the OAuth user is directly on the client account. `listAccessibleCustomers` ignores it.
- Response header `request-id` (example `request-id: 2a5Cj89VV7CNhya1DZjjrC`); "We recommend that you log these values". The REST tip also mentions `google-ads-request-id` when viewing curl headers.

## 3. search / searchStream

Sources: https://developers.google.com/google-ads/api/rest/common/search , https://developers.google.com/google-ads/api/docs/reporting/paging , https://developers.google.com/google-ads/api/rest/design/json-mappings , RPC refs SearchGoogleAdsRequest/Response, SearchGoogleAdsStreamRequest/Response

```
POST /v26/customers/CUSTOMER_ID/googleAds:search HTTP/1.1
POST /v26/customers/CUSTOMER_ID/googleAds:searchStream HTTP/1.1
Host: googleads.googleapis.com
Content-Type: application/json
Authorization: Bearer ACCESS_TOKEN
{ "query": "SELECT ... FROM ..." }
```

- `search` request fields: `query` (required), `pageToken`, `validateOnly`, `searchSettings`. `page_size` "is deprecated … returns a PAGE_SIZE_NOT_SUPPORTED error if this field is set". Pages are fixed at 10,000 rows.
- `search` response: `{"results":[...], "nextPageToken":"...", "fieldMask":"adGroupCriterion.keyword.text,adGroupCriterion.status"}` plus optional `totalResultsCount`, `summaryRow`, `queryResourceConsumption`. "next_page_token is not returned for the last page." Tokens expire "around two hours"; query must stay identical. Subsequent pages don't count against the daily operation quota.
- `searchStream` request: `query` (required), `summaryRowSetting`; "pageToken is not required by SearchStream". It is unpaged: "streams the entire result set back in a single response". REST body is a **JSON array of batch objects**, each `{"results":[...], "fieldMask":"...", "requestId":"..."}`; `queryResourceConsumption` arrives "in the final batch of results". Each `SearchGoogleAdsStreamResponse` carries `request_id` ("Response headers … with the exception of successful streaming requests").
- Rows: GAQL uses snake_case, but JSON results are lowerCamelCase nested objects, e.g.

```json
[{"results":[{"adGroupCriterion":{"resourceName":"customers/1842689525/adGroupCriteria/55771861891~10003060","keyword":{"text":"pay per click"}}}]}]
```

So `metrics.cost_micros` arrives as `results[i].metrics.costMicros`, `segments.date` as `results[i].segments.date`, `campaign_budget.amount_micros` as `results[i].campaignBudget.amountMicros`. int64 fields are JSON strings per the proto3 JSON mapping (the page defers to the protobuf JSON mapping; treat int64 as string in Go).

## 4. GAQL for campaign daily metrics

Field reference v26: https://developers.google.com/google-ads/api/fields/v26/campaign , /metrics , /segments , /campaign_budget , /customer

Campaign "Attributed resources" include `campaign_budget` and `customer`: "Fields from the above resources may be selected along with this resource in your SELECT and WHERE clauses. These fields will not segment metrics".

| Field | Type | Doc description |
|---|---|---|
| `segments.date` | DATE | "Date to which metrics apply. yyyy-MM-dd format" |
| `campaign.id` | INT64 | "The ID of the campaign." |
| `campaign.name` | STRING | |
| `campaign.status` | ENUM | ENABLED, PAUSED, REMOVED, UNKNOWN, UNSPECIFIED |
| `campaign.advertising_channel_type` | ENUM | DEMAND_GEN, DISPLAY, HOTEL, LOCAL, LOCAL_SERVICES, MULTI_CHANNEL, PERFORMANCE_MAX, SEARCH, SHOPPING, SMART, TRAVEL, VIDEO, UNKNOWN, UNSPECIFIED |
| `campaign.campaign_budget` | RESOURCE_NAME | |
| `campaign_budget.amount_micros` | INT64 | "The average daily amount … Amount is specified in micros in the account's local currency. One million micros is equivalent to one currency unit." Mutually exclusive with `total_amount_micros` (period CUSTOM_PERIOD) |
| `metrics.cost_micros` | INT64 | "The sum of your cost-per-click (CPC) and cost-per-thousand impressions (CPM) costs during this period." |
| `metrics.impressions` | INT64 | |
| `metrics.clicks` | INT64 | |
| `metrics.conversions` | DOUBLE | "only includes conversion actions which include_in_conversions_metric attribute is set to true" |
| `metrics.conversions_value` | DOUBLE | same filter |
| `metrics.all_conversions` | DOUBLE | "includes all conversions regardless of the value of include_in_conversions_metric" |
| `metrics.all_conversions_value` | DOUBLE | |
| `customer.currency_code` | STRING | "Immutable. The currency in which the account operates. A subset of the currency codes from the ISO 4217 standard" |
| `customer.time_zone` | STRING | "Immutable. The local timezone ID of the customer." |

Date ranges (https://developers.google.com/google-ads/api/docs/query/date-ranges): `segments.date BETWEEN '2026-01-01' AND '2026-01-31'` or comparisons (`>=`, `<=`), ISO `YYYY-MM-DD` or `YYYYMMDD`; `segments.date DURING LAST_30_DAYS` where DURING "supports only predefined date ranges" (TODAY, YESTERDAY, LAST_7_DAYS, LAST_BUSINESS_WEEK, THIS_MONTH, LAST_MONTH, LAST_14_DAYS, LAST_30_DAYS, THIS_WEEK_SUN_TODAY, THIS_WEEK_MON_TODAY, LAST_WEEK_SUN_SAT, LAST_WEEK_MON_SUN). "All date ranges and time periods are evaluated relative to the serving account's time zone (customer.time_zone), rather than UTC." Daily granularity lookback: 37 months, else `DateRangeError.REQUESTED_DATE_GRANULARITY_NOT_SUPPORTED`.

## 5. Account hierarchy

Sources: https://developers.google.com/google-ads/api/docs/account-management/get-account-hierarchy , https://developers.google.com/google-ads/api/docs/account-management/listing-accounts , https://developers.google.com/google-ads/api/fields/v26/customer_client

Doc query (run against a manager, with `login-customer-id` = that manager):
```
SELECT customer_client.client_customer, customer_client.level, customer_client.manager,
  customer_client.descriptive_name, customer_client.currency_code, customer_client.time_zone,
  customer_client.id, customer_client.status
FROM customer_client
WHERE customer_client.level <= 1 AND customer_client.status = 'ENABLED'
```
Fields: `id` INT64; `descriptive_name` STRING; `level` INT64 ("Distance between given customer and client. For self link, the level value will be 0"); `manager` BOOLEAN; `currency_code` STRING ('USD', 'EUR'); `time_zone` STRING (CLDR, e.g. America/Los_Angeles); `status` ENUM CANCELED, CLOSED, ENABLED, SUSPENDED, UNKNOWN, UNSPECIFIED; also `client_customer` RESOURCE_NAME, `hidden` BOOLEAN, `test_account` BOOLEAN. Docs caution that traversing deep hierarchies recursively is expensive; samples do a breadth-first walk with `level <= 1`.

```
GET https://googleads.googleapis.com/v26/customers:listAccessibleCustomers
Authorization: Bearer ...        (no body; login-customer-id ignored)
→ {"resourceNames": ["customers/1234567890", ...]}
```
"Resource name of customers directly accessible by the user authenticating the call" – only accounts where the user is added directly, not the whole hierarchy.

## 6. Conversion actions

Sources: https://developers.google.com/google-ads/api/fields/v26/conversion_action , https://developers.google.com/google-ads/api/docs/conversions/reporting

`conversion_action.resource_name` (`customers/{customer_id}/conversionActions/{conversion_action_id}`), `.id` INT64, `.name` STRING, `.type` ENUM (AD_CALL, WEBPAGE, UPLOAD_CLICKS, …), `.category` ENUM (ADD_TO_CART, BEGIN_CHECKOUT, BOOK_APPOINTMENT, CONTACT, CONVERTED_LEAD, …), `.status` ENUM ENABLED ("Conversions will be recorded"), HIDDEN, REMOVED, UNKNOWN, UNSPECIFIED; `.counting_type` ENUM MANY_PER_CLICK, ONE_PER_CLICK, UNKNOWN, UNSPECIFIED; `.include_in_conversions_metric` BOOLEAN; `.primary_for_goal` BOOLEAN.

Per-action metrics: `segments.conversion_action` (RESOURCE_NAME), `segments.conversion_action_name` (STRING), `segments.conversion_action_category` (ENUM). Doc example:
```
SELECT campaign.name, segments.conversion_action, metrics.conversions, metrics.conversions_value
FROM campaign WHERE segments.date DURING LAST_30_DAYS
```
"Including segments.conversion_action in the SELECT clause changes the granularity of the query results to one row per conversion action per campaign, and rows with zero conversions are excluded from the response." "Rows in reports whose metrics are all zero … won't be returned." Custom columns are not available via API. Whether cost/clicks are selectable alongside `segments.conversion_action` on `campaign`: **unconfirmed** (query the two separately).

## 7. Errors

Sources: https://developers.google.com/google-ads/api/docs/get-started/handle-errors , https://developers.google.com/google-ads/api/docs/best-practices/understand-api-errors , https://developers.google.com/google-ads/api/docs/common-errors , RPC refs GoogleAdsFailure, GoogleAdsError, QuotaErrorEnum, QuotaErrorDetails

REST body (the REST example wraps this under a top-level `"error"` key per google.rpc.Status; the docs show the Status fields):
```json
{"code":3,"message":"Request contains an invalid argument.","details":[{
  "@type":"type.googleapis.com/google.ads.googleads.v26.errors.GoogleAdsFailure",
  "errors":[{"errorCode":{"requestError":"REQUIRED_FIELD_MISSING"},"message":"Required field is missing",
    "trigger":{"stringValue":""},
    "location":{"fieldPathElements":[{"fieldName":"ad_group","index":0},{"fieldName":"name"}]}}],
  "requestId":"unique_request_id_12345"}]}
```
`errorCode` is a one-key object whose key names the error family (`authenticationError`, `authorizationError`, `quotaError`, `requestError`, `queryError`, …). `GoogleAdsError.details` (ErrorDetails) may carry `quotaErrorDetails` with `rateScope`, `rateName` ("Requests per account"), `retryDelay` (Duration: "Backoff period that customers should wait before sending next request").

gRPC→HTTP mapping: INVALID_ARGUMENT 400, UNAUTHENTICATED 401, PERMISSION_DENIED 403, NOT_FOUND 404, RESOURCE_EXHAUSTED 429, INTERNAL 500, UNAVAILABLE 503, DEADLINE_EXCEEDED 504. Retry only UNAVAILABLE, DEADLINE_EXCEEDED, INTERNAL, UNKNOWN, ABORTED with exponential backoff + jitter ("wait 1s, then 2s, then 4s").

Key codes:
- `AuthenticationError.NOT_ADS_USER`, `OAUTH_TOKEN_INVALID`, `OAUTH_TOKEN_EXPIRED`, `OAUTH_TOKEN_REVOKED` (re-run OAuth), `CUSTOMER_NOT_FOUND` ("Wait an initial five minutes, then retry every 30 seconds" for new accounts).
- `AuthorizationError.USER_PERMISSION_DENIED` ("manager's customer ID must be set in the login-customer-id header"), `CLOUD_PROJECT_NOT_APPROVED_FOR_PRODUCTION` (v25+; Test-access project hitting a production account; v24 and earlier returned `ACTION_NOT_PERMITTED`), `CUSTOMER_NOT_ENABLED`, `MISSING_TOS`, `PROJECT_DISABLED`.
- `QuotaError.RESOURCE_EXHAUSTED` "Too many requests." (daily operation quota; HTTP 429), `QuotaError.RESOURCE_TEMPORARILY_EXHAUSTED` "Too many requests in a short amount of time." (QPS), `ACCESS_PROHIBITED`.
- `RequestError.PAGE_SIZE_NOT_SUPPORTED`; expired page token → error, restart the query.
- `request-id` header on every response except successful streams (use per-batch `requestId` there).

## 8. Quotas and backoff

Sources: https://developers.google.com/google-ads/api/docs/access-levels , https://developers.google.com/google-ads/api/docs/best-practices/quotas , https://developers.google.com/google-ads/api/docs/best-practices/rate-limits

Access levels are per Google Cloud project, "sliding 24 hour" window: Test 15,000 ops/day (test accounts only); **Explorer** 2,880 ops/day production + 15,000 test; **Basic** 15,000 ops/day; **Standard** unlimited (manual audit, ~10 business days; Basic needs brand verification). "A Search or SearchStream request counts as one operation … One SearchStream request counts as one API operation irrespective of the number of batches." Paged follow-ups with a valid token are free; failed requests that return a GoogleAdsFailure still count. Rate limiting is token-bucket QPS per customer ID and per Cloud project, limit "varies depending on the overall server load"; violations return `RESOURCE_TEMPORARILY_EXHAUSTED`. Guidance: "exponential backoff policy with randomized jitter … waiting 2 seconds, then 4 seconds, then 8 seconds, plus a small random delay"; cap concurrency; client-side rate limiter. Honor `retryDelay` if present. Response size limit 64 MB (gRPC; REST unconfirmed) – use searchStream or fewer fields.

## 9. Data freshness and revisions

The API docs defer to the Google Ads Help article https://support.google.com/google-ads/answer/2544985 (linked from https://developers.google.com/google-ads/api/docs/conversions/reporting): "clicks, impressions, and cost are subject to a 1-hour data freshness Service Level Objective"; last-click conversions ~3 hours, other attribution models ~15 hours (non-last-click conversions for Sunday ready Monday afternoon/evening local time); search terms/geo next-day; impression share 3 days. Revisions: "Your reporting metrics may occasionally be updated one or more days after an event occurs" due to "Invalid traffic … removed retroactively", "Late-arriving conversions … several days after the initial ad interaction, especially for advertisers with longer conversion windows", and "End-of-month adjustments: Billable events may be adjusted during the billing cycle". Data retained ~11 years; daily segments queryable 37 months back. An exact "numbers are final after N days" figure is **unconfirmed**; the `ConversionOrAdjustmentLagBucket` enum goes up to 90–145 days, so conversions can keep changing for months. Practical rule for ADX: re-pull at least the last 3 days every run, and periodically re-pull ~30–90 days for conversions.

## Go client checklist

- Base `https://googleads.googleapis.com/v26`; headers `Authorization: Bearer`, `login-customer-id` (manager CID, digits only) when using a manager; skip `developer-token`.
- Use `googleAds:searchStream`; decode a JSON array of `{results, fieldMask, requestId}` batches; int64 as strings; micros ÷ 1,000,000.
- Treat 429 + `quotaError` as retryable with backoff (and `retryDelay` if present); 401/403 auth codes as non-retryable "reconnect" conditions; 5xx retry with backoff.
- Hierarchy: `customers:listAccessibleCustomers` → for each manager, `customer_client` with `level <= 1` BFS.

## Keyword ideas (B2.2)

`POST /v26/customers/{customerId}:generateKeywordIdeas` with `{"keywordSeed":{"keywords":[...]},"geoTargetConstants":["geoTargetConstants/2784"],"language":"languageConstants/1000","keywordPlanNetwork":"GOOGLE_SEARCH","includeAdultKeywords":false,"pageSize":200}`. Country constants are 2000 plus the ISO 3166-1 numeric code (UAE 784 → 2784); languages: English 1000, Arabic 1019. Each result is `{"text":"...","keywordIdeaMetrics":{"avgMonthlySearches":"12100","competition":"HIGH","competitionIndex":"87","lowTopOfPageBidMicros":"...","highTopOfPageBidMicros":"..."}}`, int64s as strings, bids in micros of the customer's currency; `keywordIdeaMetrics` is absent when Google has no figure. The request shape is from the published reference and `providers/googleads/keywords_test.go`; it is not verified against the live API because no OAuth client exists here. The same login-customer-id and token rules as the read adapter apply.
