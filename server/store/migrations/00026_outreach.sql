-- +goose Up
-- An objection is something the customer pushed back on, tied to the
-- enquiry it came up in, with where it was heard.
CREATE TABLE objections (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id    INTEGER  NOT NULL REFERENCES people (id),
    enquiry_id   INTEGER  REFERENCES enquiries (id),
    kind         TEXT     NOT NULL CHECK (kind IN ('price', 'location', 'timing', 'payment', 'trust', 'unit', 'other')),
    text         TEXT     NOT NULL,
    source       TEXT     NOT NULL,
    evidence_id  INTEGER  REFERENCES evidence (id),
    recorded_by  INTEGER  REFERENCES users (id),
    recorded_at  DATETIME NOT NULL,
    resolved_at  DATETIME,
    resolution   TEXT     NOT NULL DEFAULT ''
);
CREATE INDEX objections_person ON objections (person_id, resolved_at);

-- A suppression is an opt-out: a channel (or all) the person must not be
-- contacted on. Checked at dispatch, every time, including retries.
CREATE TABLE contact_suppressions (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id    INTEGER  NOT NULL REFERENCES people (id),
    channel      TEXT     NOT NULL CHECK (channel IN ('all', 'whatsapp', 'sms', 'email', 'call')),
    reason       TEXT     NOT NULL,
    source       TEXT     NOT NULL,
    since        DATETIME NOT NULL,
    lifted_at    DATETIME,
    lifted_by    INTEGER  REFERENCES users (id),
    lift_reason  TEXT     NOT NULL DEFAULT ''
);
CREATE INDEX contact_suppressions_person ON contact_suppressions (person_id, lifted_at);

-- The outreach configuration: which CRM fields ADX owns and which the CRM
-- owns, and the messaging mode per channel. Versioned like policies; no row
-- means unconfigured, which sends nothing and treats every field edit as a
-- conflict for a person to look at.
CREATE TABLE outreach_configs (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

-- The CRM field outbox: one row per value ADX wants a CRM field to have.
-- A later observation of the same value is an echo and confirms it; a
-- different value is a conflict or, when the CRM owns the field, supersedes
-- it according to the configured ownership.
CREATE TABLE crm_field_outbox (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id       INTEGER  NOT NULL REFERENCES people (id),
    account_id      INTEGER  NOT NULL REFERENCES accounts (id),
    external_id     TEXT     NOT NULL,
    field           TEXT     NOT NULL,
    value           TEXT     NOT NULL,
    state           TEXT     NOT NULL CHECK (state IN ('pending', 'held', 'sent', 'confirmed', 'conflict', 'superseded', 'failed')),
    reason          TEXT     NOT NULL DEFAULT '',
    attempts        INTEGER  NOT NULL DEFAULT 0,
    idempotency_key TEXT     NOT NULL,
    conflict        TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(conflict)),
    created_by      INTEGER  REFERENCES users (id),
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    sent_at         DATETIME,
    confirmed_at    DATETIME,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX crm_field_outbox_state ON crm_field_outbox (workspace_id, state);
CREATE INDEX crm_field_outbox_target ON crm_field_outbox (account_id, external_id, field);

-- Message drafts. A draft is text with its claim check; it is sent only
-- through a configured channel mode, after approval where the mode says so,
-- and never to a suppressed person.
CREATE TABLE message_drafts (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id       INTEGER  NOT NULL REFERENCES people (id),
    enquiry_id      INTEGER  REFERENCES enquiries (id),
    channel         TEXT     NOT NULL CHECK (channel IN ('whatsapp', 'sms', 'email')),
    language        TEXT     NOT NULL DEFAULT 'en',
    body            TEXT     NOT NULL,
    claims          TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(claims)),
    state           TEXT     NOT NULL CHECK (state IN ('draft', 'approved', 'held', 'sent', 'suppressed', 'failed')),
    reason          TEXT     NOT NULL DEFAULT '',
    attempts        INTEGER  NOT NULL DEFAULT 0,
    idempotency_key TEXT     NOT NULL,
    provider_ref    TEXT     NOT NULL DEFAULT '',
    created_by      INTEGER  REFERENCES users (id),
    approved_by     INTEGER  REFERENCES users (id),
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    sent_at         DATETIME,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX message_drafts_person ON message_drafts (person_id, created_at);

-- +goose Down
DROP TABLE message_drafts;
DROP TABLE crm_field_outbox;
DROP TABLE outreach_configs;
DROP TABLE contact_suppressions;
DROP TABLE objections;
