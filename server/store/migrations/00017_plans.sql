-- +goose Up
-- A media plan for a brand or project. Its versions are immutable JSON
-- documents (core/plans.PlanDocument): typed allocations, asset, page and
-- measurement requirements, test hypotheses, evidence references and
-- assumptions. A version is validated in code when it is saved and keeps
-- the result. Acceptance is content acceptance by an admin and is terminal:
-- a change is a new version. Nothing here authorizes spend.
CREATE TABLE plans (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id     INTEGER  NOT NULL REFERENCES brands (id),
    project_id   INTEGER  REFERENCES projects (id),
    title        TEXT     NOT NULL,
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
CREATE INDEX plans_brand ON plans (workspace_id, brand_id, project_id);

CREATE TABLE plan_versions (
    id            INTEGER  PRIMARY KEY,
    plan_id       INTEGER  NOT NULL REFERENCES plans (id),
    version       INTEGER  NOT NULL,
    document      TEXT     NOT NULL CHECK (json_valid(document)),
    state         TEXT     NOT NULL CHECK (state IN ('draft', 'validating', 'ready', 'needs_input', 'invalid', 'accepted')),
    validation    TEXT     NOT NULL CHECK (json_valid(validation)),
    source        TEXT     NOT NULL CHECK (source IN ('manual', 'generated', 'imported')),
    note          TEXT     NOT NULL DEFAULT '',
    created_by    INTEGER  REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    accepted_by   INTEGER  REFERENCES users (id),
    accepted_at   DATETIME,
    UNIQUE (plan_id, version)
);

-- +goose Down
DROP TABLE plan_versions;
DROP TABLE plans;
