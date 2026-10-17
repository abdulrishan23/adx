-- +goose Up
-- A lead is one GoHighLevel contact as ADX last saw it. It is keyed by the
-- provider's contact id, never by phone or email: two contacts sharing a
-- phone stay two leads until phase 4's person resolution says otherwise.
-- status_raw is the status as the CRM gave it; when a scan returns the
-- contact without the status field, the last known value stays and
-- status_missing_since records that. removed_at is set only by a complete
-- full scan that did not see the contact.
CREATE TABLE leads (
    id                   INTEGER  PRIMARY KEY,
    workspace_id         INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id           INTEGER  NOT NULL REFERENCES accounts (id),
    external_id          TEXT     NOT NULL,
    first_name           TEXT,
    last_name            TEXT,
    email                TEXT,
    phone                TEXT,
    phone_e164           TEXT,
    dial_code            TEXT,
    source_raw           TEXT,
    project_raw          TEXT,
    project_id           INTEGER  REFERENCES projects (id),
    status_raw           TEXT,
    status_observed_at   DATETIME,
    status_missing_since DATETIME,
    stage                TEXT,
    mapping_version      INTEGER,
    provider_added_at    DATETIME,
    provider_updated_at  DATETIME,
    first_seen_at        DATETIME NOT NULL,
    last_seen_at         DATETIME NOT NULL,
    last_seen_run_id     INTEGER  REFERENCES runs (id),
    evidence_id          INTEGER  REFERENCES evidence (id),
    removed_at           DATETIME,
    UNIQUE (account_id, external_id)
);
CREATE INDEX leads_workspace_seen ON leads (workspace_id, last_seen_at);
CREATE INDEX leads_phone ON leads (workspace_id, phone_e164);
CREATE INDEX leads_stage ON leads (workspace_id, stage);
CREATE INDEX leads_status ON leads (workspace_id, status_raw);

-- What the CRM recorded about where a lead came from, verbatim, plus ADX's
-- campaign match. match_level follows reference.md: click_id, platform_lead_id,
-- hashed_contact, utm, none. Confidence is ADX's own and is inferred evidence.
CREATE TABLE lead_attributions (
    id                   INTEGER  PRIMARY KEY,
    lead_id              INTEGER  NOT NULL REFERENCES leads (id),
    observed_at          DATETIME NOT NULL,
    is_first             INTEGER  NOT NULL DEFAULT 0,
    utm_source           TEXT,
    utm_medium           TEXT,
    utm_campaign         TEXT,
    utm_content          TEXT,
    utm_term             TEXT,
    gclid                TEXT,
    fbclid               TEXT,
    referrer             TEXT,
    landing_url          TEXT,
    match_level          TEXT     NOT NULL CHECK (match_level IN ('click_id', 'platform_lead_id', 'hashed_contact', 'utm', 'none')),
    match_confidence     TEXT     NOT NULL CHECK (match_confidence IN ('high', 'medium', 'low', 'none')),
    matched_platform     TEXT,
    matched_campaign_ref TEXT,
    payload_hash         TEXT     NOT NULL,
    UNIQUE (lead_id, payload_hash)
);

-- The funnel configuration: where the status and project live in the CRM,
-- which raw status means which ADX stage, and which raw source means which
-- ADX source. One versioned document per workspace; no row means
-- unconfigured, and unconfigured leaves every lead's stage null.
CREATE TABLE funnel_mappings (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

-- +goose Down
DROP TABLE funnel_mappings;
DROP TABLE lead_attributions;
DROP TABLE leads;
