-- +goose Up
-- Files ADX stores: renders (photos), logos, fonts and rendered outputs. The
-- bytes live in the asset store under a content-addressed key (core/assets);
-- this row is the metadata. Nothing is ever overwritten: a changed file is a
-- new row with a new key.
CREATE TABLE assets (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    project_id    INTEGER  REFERENCES projects (id),
    kind          TEXT     NOT NULL CHECK (kind IN ('render', 'logo', 'font', 'output', 'other')),
    name          TEXT     NOT NULL,
    key           TEXT     NOT NULL,
    sha256        TEXT     NOT NULL,
    size          INTEGER  NOT NULL,
    content_type  TEXT     NOT NULL,
    uploaded_by   INTEGER  REFERENCES users (id),
    created_at    DATETIME NOT NULL
);
CREATE INDEX assets_workspace_project ON assets (workspace_id, project_id, kind);

-- A brand kit per project: logos, fonts and colours as a versioned JSON
-- document (core/assets.BrandKit). A new version on every save.
CREATE TABLE brand_kits (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    project_id   INTEGER  NOT NULL REFERENCES projects (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (project_id, version)
);

-- A creative is one ad in one format and language. Its versions are the
-- editable layout documents; each version is rendered to an immutable
-- output and reviewed on its own. Review binds to the version's rendered
-- checksum, so editing after approval means a new, unapproved version.
CREATE TABLE creatives (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    project_id   INTEGER  REFERENCES projects (id),
    name         TEXT     NOT NULL,
    format       TEXT     NOT NULL CHECK (format IN ('9:16', '4:5', '1:1', '1.91:1')),
    language     TEXT     NOT NULL CHECK (language IN ('en', 'ar')),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
CREATE INDEX creatives_workspace ON creatives (workspace_id, project_id);

CREATE TABLE creative_versions (
    id            INTEGER  PRIMARY KEY,
    creative_id   INTEGER  NOT NULL REFERENCES creatives (id),
    version       INTEGER  NOT NULL,
    document      TEXT     NOT NULL CHECK (json_valid(document)),
    review_state  TEXT     NOT NULL CHECK (review_state IN ('draft', 'rendered', 'in_review', 'approved', 'revise')),
    renderer      TEXT,
    output_key    TEXT,
    output_sha256 TEXT,
    output_size   INTEGER,
    output_type   TEXT,
    rendered_at   DATETIME,
    created_by    INTEGER  REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    UNIQUE (creative_id, version)
);

-- One decision per reviewer per version. The approval names the exact
-- output checksum it accepted. It never grants delivery (core/assets).
CREATE TABLE creative_reviews (
    id            INTEGER  PRIMARY KEY,
    version_id    INTEGER  NOT NULL REFERENCES creative_versions (id),
    decision      TEXT     NOT NULL CHECK (decision IN ('approved', 'revise')),
    output_sha256 TEXT     NOT NULL,
    note          TEXT     NOT NULL DEFAULT '',
    decided_by    INTEGER  NOT NULL REFERENCES users (id),
    decided_at    DATETIME NOT NULL
);
CREATE INDEX creative_reviews_version ON creative_reviews (version_id);

-- +goose Down
DROP TABLE creative_reviews;
DROP TABLE creative_versions;
DROP TABLE creatives;
DROP TABLE brand_kits;
DROP TABLE assets;
