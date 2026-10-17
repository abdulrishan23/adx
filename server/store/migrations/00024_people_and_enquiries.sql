-- +goose Up
-- A person is who; an enquiry is what they asked about (a project). One
-- person can have several enquiries. Outside records (a CRM contact, a
-- form submission) are linked to a person with the evidence that matched
-- them; a link can be reversed, a merge can be undone, and a match that is
-- not exact stays visible as a candidate for a person to decide.
CREATE TABLE people (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    display_name   TEXT     NOT NULL DEFAULT '',
    primary_phone  TEXT,
    primary_email  TEXT,
    country        TEXT,
    language       TEXT,
    notes          TEXT     NOT NULL DEFAULT '',
    merged_into_id INTEGER  REFERENCES people (id),
    merge_record   TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(merge_record)),
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL
);
CREATE INDEX people_phone ON people (workspace_id, primary_phone);
CREATE INDEX people_email ON people (workspace_id, primary_email);

-- Every identifier a person was seen with, normalized, with where it came
-- from. stated: the customer gave it. inferred: an enrichment provider.
CREATE TABLE person_identifiers (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id     INTEGER  NOT NULL REFERENCES people (id),
    kind          TEXT     NOT NULL CHECK (kind IN ('phone', 'email', 'external')),
    value         TEXT     NOT NULL,
    class         TEXT     NOT NULL CHECK (class IN ('stated', 'inferred')),
    source        TEXT     NOT NULL,
    evidence_id   INTEGER  REFERENCES evidence (id),
    first_seen_at DATETIME NOT NULL,
    UNIQUE (person_id, kind, value)
);
CREATE INDEX person_identifiers_value ON person_identifiers (workspace_id, kind, value);

CREATE TABLE enquiries (
    id               INTEGER  PRIMARY KEY,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id        INTEGER  NOT NULL REFERENCES people (id),
    project_id       INTEGER  REFERENCES projects (id),
    state            TEXT     NOT NULL CHECK (state IN ('open', 'closed')),
    stage            TEXT,
    source_kind      TEXT     NOT NULL,
    source_ref       TEXT     NOT NULL,
    first_contact_at DATETIME NOT NULL,
    last_activity_at DATETIME NOT NULL,
    created_at       DATETIME NOT NULL
);
CREATE INDEX enquiries_person ON enquiries (person_id, project_id);
CREATE INDEX enquiries_project ON enquiries (workspace_id, project_id, state);

-- A link ties an outside record to a person with the evidence that matched.
CREATE TABLE person_links (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id       INTEGER  NOT NULL REFERENCES people (id),
    kind            TEXT     NOT NULL CHECK (kind IN ('lead', 'form_submission')),
    ref             TEXT     NOT NULL,
    match_level     TEXT     NOT NULL CHECK (match_level IN ('exact', 'new', 'manual', 'merged')),
    match_evidence  TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(match_evidence)),
    linked_at       DATETIME NOT NULL,
    linked_by       INTEGER  REFERENCES users (id),
    reversed_at     DATETIME,
    reversed_by     INTEGER  REFERENCES users (id),
    reversal_reason TEXT     NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX person_links_active ON person_links (workspace_id, kind, ref) WHERE reversed_at IS NULL;
CREATE INDEX person_links_person ON person_links (person_id);

-- A candidate is a match that was not exact: the record went to person_id
-- (usually a new person) and might belong to candidate_id instead.
CREATE TABLE match_candidates (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id     INTEGER  NOT NULL REFERENCES people (id),
    candidate_id  INTEGER  NOT NULL REFERENCES people (id),
    reason        TEXT     NOT NULL CHECK (json_valid(reason)),
    state         TEXT     NOT NULL CHECK (state IN ('open', 'accepted', 'rejected')),
    created_at    DATETIME NOT NULL,
    decided_by    INTEGER  REFERENCES users (id),
    decided_at    DATETIME,
    note          TEXT     NOT NULL DEFAULT '',
    UNIQUE (person_id, candidate_id)
);
CREATE INDEX match_candidates_open ON match_candidates (workspace_id, state);

-- Attributes carry their class: stated by the customer, or enriched by a
-- provider. A stated value is never overwritten by an enriched one; both
-- are kept and a disagreement is shown.
CREATE TABLE person_attributes (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id     INTEGER  NOT NULL REFERENCES people (id),
    key           TEXT     NOT NULL,
    value         TEXT     NOT NULL,
    class         TEXT     NOT NULL CHECK (class IN ('stated', 'enriched')),
    source        TEXT     NOT NULL,
    evidence_id   INTEGER  REFERENCES evidence (id),
    observed_at   DATETIME NOT NULL,
    superseded_at DATETIME
);
CREATE INDEX person_attributes_person ON person_attributes (person_id, key);

-- +goose Down
DROP TABLE person_attributes;
DROP TABLE match_candidates;
DROP TABLE person_links;
DROP TABLE enquiries;
DROP TABLE person_identifiers;
DROP TABLE people;
