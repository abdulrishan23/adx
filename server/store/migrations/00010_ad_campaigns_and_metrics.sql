-- +goose Up
-- Campaigns as the ad platforms describe them. One row per platform
-- campaign, keyed by the account and the platform's id. The platform's own
-- status and type are kept raw; budgets are minor units in the account's
-- currency (Google reports micros, Meta minor units; both are converted on
-- import). project_id is ADX's reading of the campaign name, filled by B1.3.
CREATE TABLE ad_campaigns (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id     INTEGER  NOT NULL REFERENCES accounts (id),
    platform       TEXT     NOT NULL CHECK (platform IN ('google_ads', 'meta')),
    external_id    TEXT     NOT NULL,
    name           TEXT     NOT NULL,
    status         TEXT     NOT NULL,
    channel_type   TEXT,
    budget_minor   INTEGER,
    budget_period  TEXT,
    currency       TEXT,
    project_id     INTEGER  REFERENCES projects (id),
    first_seen_at  DATETIME NOT NULL,
    last_seen_at   DATETIME NOT NULL,
    removed_at     DATETIME,
    UNIQUE (account_id, external_id)
);
CREATE INDEX ad_campaigns_workspace ON ad_campaigns (workspace_id, platform);

-- One row per campaign per day, in the account's own time zone, as the
-- platform reported it when ADX last read it. Money is minor units in
-- currency. conversions and leads are the platform's own claims and are
-- never added across platforms (reference.md). A null is "the platform
-- didn't say", never zero. observed_at and run_id say which read produced
-- the row; the platform revises recent days, so rows are replaced on re-read.
CREATE TABLE campaign_daily_metrics (
    id                      INTEGER  PRIMARY KEY,
    workspace_id            INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id              INTEGER  NOT NULL REFERENCES accounts (id),
    campaign_id             INTEGER  NOT NULL REFERENCES ad_campaigns (id),
    platform                TEXT     NOT NULL,
    day                     TEXT     NOT NULL,
    currency                TEXT     NOT NULL,
    cost_minor              INTEGER  NOT NULL,
    impressions             INTEGER  NOT NULL,
    clicks                  INTEGER  NOT NULL,
    conversions             REAL,
    conversions_value_minor INTEGER,
    all_conversions         REAL,
    platform_leads          REAL,
    reach                   INTEGER,
    observed_at             DATETIME NOT NULL,
    run_id                  INTEGER  REFERENCES runs (id),
    UNIQUE (campaign_id, day)
);
CREATE INDEX campaign_daily_metrics_day ON campaign_daily_metrics (workspace_id, day);
CREATE INDEX campaign_daily_metrics_account_day ON campaign_daily_metrics (account_id, day);

-- Google's conversion actions, so a "conversion" can be named and so B4/B5
-- outcome uploads know where to send a Done-Deal.
CREATE TABLE conversion_actions (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id    INTEGER  NOT NULL REFERENCES accounts (id),
    external_id   TEXT     NOT NULL,
    name          TEXT     NOT NULL,
    type          TEXT,
    category      TEXT,
    status        TEXT,
    counting_type TEXT,
    include_in_conversions INTEGER NOT NULL DEFAULT 1,
    last_seen_at  DATETIME NOT NULL,
    UNIQUE (account_id, external_id)
);

-- What each account's data is good for: through which day a complete read
-- exists, when the last successful read ran, and whether the last read was
-- partial. Freshness on screens comes from here.
CREATE TABLE account_freshness (
    account_id        INTEGER  PRIMARY KEY REFERENCES accounts (id),
    workspace_id      INTEGER  NOT NULL REFERENCES workspaces (id),
    last_success_at   DATETIME,
    observed_through  TEXT,
    last_run_id       INTEGER  REFERENCES runs (id),
    last_partial      INTEGER  NOT NULL DEFAULT 0,
    last_error        TEXT
);

-- +goose Down
DROP TABLE account_freshness;
DROP TABLE conversion_actions;
DROP TABLE campaign_daily_metrics;
DROP TABLE ad_campaigns;
