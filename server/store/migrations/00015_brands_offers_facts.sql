-- +goose Up
-- A brand is the developer or client whose projects ADX markets (Reportage),
-- or a competitor ADX only observes. Everything a project says about itself
-- hangs off its brand, so evidence about one brand never reaches another's
-- drafts (core/brands).
CREATE TABLE brands (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    name         TEXT     NOT NULL,
    kind         TEXT     NOT NULL CHECK (kind IN ('own', 'competitor')),
    notes        TEXT     NOT NULL DEFAULT '',
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, name COLLATE NOCASE)
);

-- A project belongs to at most one brand. Existing projects keep their ids
-- and start unassigned; an admin assigns them.
ALTER TABLE projects ADD COLUMN brand_id INTEGER REFERENCES brands (id);
CREATE INDEX projects_brand ON projects (brand_id);

-- An offer is a commercial proposition with a life: "10% off until the end
-- of Ramadan". Its versions are immutable documents with effective dates;
-- saving a change is a new version, and whatever used the old one is listed
-- as affected rather than silently updated.
CREATE TABLE offers (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id     INTEGER  NOT NULL REFERENCES brands (id),
    project_id   INTEGER  REFERENCES projects (id),
    name         TEXT     NOT NULL,
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL
);
CREATE INDEX offers_brand ON offers (workspace_id, brand_id, project_id);

CREATE TABLE offer_versions (
    id             INTEGER  PRIMARY KEY,
    offer_id       INTEGER  NOT NULL REFERENCES offers (id),
    version        INTEGER  NOT NULL,
    document       TEXT     NOT NULL CHECK (json_valid(document)),
    effective_from DATE     NOT NULL,
    effective_to   DATE,
    state          TEXT     NOT NULL CHECK (state IN ('draft', 'approved', 'withdrawn')),
    note           TEXT     NOT NULL DEFAULT '',
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    approved_by    INTEGER  REFERENCES users (id),
    approved_at    DATETIME,
    UNIQUE (offer_id, version)
);

-- A creative may be built on an offer; the version it was built on is kept
-- so a later offer change can name it as affected.
ALTER TABLE creatives ADD COLUMN offer_id INTEGER REFERENCES offers (id);
ALTER TABLE creatives ADD COLUMN offer_version INTEGER;
CREATE INDEX creatives_offer ON creatives (offer_id);

-- A fact is one assertion about a brand or project: "price.from = 1,200,000
-- AED from 2026-10-01". Nothing is ever edited or deleted: a correction is a
-- new row and the old one is retired with a reason. Class says how much to
-- trust it: approved (an admin signed off), needs_review (a person entered
-- it), inferred (ADX derived it from evidence; never approved on its own),
-- retired (no longer current). Two current facts for the same key with
-- different values and overlapping dates are a conflict, and a conflicting
-- fact cannot be approved until an admin resolves it.
CREATE TABLE facts (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id       INTEGER  NOT NULL REFERENCES brands (id),
    project_id     INTEGER  REFERENCES projects (id),
    key            TEXT     NOT NULL,
    kind           TEXT     NOT NULL CHECK (kind IN ('text', 'money', 'number', 'percent', 'date')),
    value          TEXT     NOT NULL,
    currency       TEXT,
    effective_from DATE     NOT NULL,
    effective_to   DATE,
    class          TEXT     NOT NULL CHECK (class IN ('approved', 'needs_review', 'inferred', 'retired')),
    source_kind    TEXT     NOT NULL CHECK (source_kind IN ('manual', 'document', 'evidence')),
    source_ref     TEXT     NOT NULL DEFAULT '',
    evidence_id    INTEGER  REFERENCES evidence (id),
    note           TEXT     NOT NULL DEFAULT '',
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    approved_by    INTEGER  REFERENCES users (id),
    approved_at    DATETIME,
    retired_at     DATETIME,
    retired_reason TEXT     NOT NULL DEFAULT '',
    supersedes_id  INTEGER  REFERENCES facts (id)
);
CREATE INDEX facts_brand_key ON facts (workspace_id, brand_id, key, class);

CREATE TABLE fact_conflicts (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id     INTEGER  NOT NULL REFERENCES brands (id),
    key          TEXT     NOT NULL,
    fact_a_id    INTEGER  NOT NULL REFERENCES facts (id),
    fact_b_id    INTEGER  NOT NULL REFERENCES facts (id),
    state        TEXT     NOT NULL CHECK (state IN ('open', 'resolved')),
    opened_at    DATETIME NOT NULL,
    resolved_by  INTEGER  REFERENCES users (id),
    resolved_at  DATETIME,
    winner_id    INTEGER  REFERENCES facts (id),
    resolution   TEXT     NOT NULL DEFAULT '',
    UNIQUE (fact_a_id, fact_b_id)
);
CREATE INDEX fact_conflicts_brand ON fact_conflicts (workspace_id, brand_id, state);

-- +goose Down
DROP TABLE fact_conflicts;
DROP TABLE facts;
DROP INDEX creatives_offer;
ALTER TABLE creatives DROP COLUMN offer_version;
ALTER TABLE creatives DROP COLUMN offer_id;
DROP TABLE offer_versions;
DROP TABLE offers;
DROP INDEX projects_brand;
ALTER TABLE projects DROP COLUMN brand_id;
DROP TABLE brands;
