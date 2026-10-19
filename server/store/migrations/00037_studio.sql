-- +goose Up
-- Creative production, ported from Creative Studio. A project's creatives
-- are concepts, each with ads in four sizes, two languages and with or
-- without a CTA. Every published picture of an ad is an immutable version,
-- and a review belongs to the version it was given on, so an edited ad
-- goes back to waiting (docs/screens.md).

-- Uploaded files: ad pictures, previews, renders, logos and fonts. The bytes
-- live on disk under their SHA-256, so the same upload is stored once.
CREATE TABLE files (
    id           INTEGER PRIMARY KEY,
    sha256       TEXT     NOT NULL UNIQUE,
    content_type TEXT     NOT NULL,
    bytes        INTEGER  NOT NULL,
    width        INTEGER,
    height       INTEGER,
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL
);

-- A project's creative settings. The review key is the public board's link;
-- rotating it stops the old link working.
CREATE TABLE creative_projects (
    project_id    INTEGER  PRIMARY KEY REFERENCES projects (id),
    prefix        TEXT     NOT NULL CHECK (prefix GLOB '[A-Z0-9]*' AND prefix != ''),
    kicker        TEXT     NOT NULL DEFAULT '',
    page_name     TEXT     NOT NULL,
    review_key    TEXT     NOT NULL UNIQUE,
    review_active INTEGER  NOT NULL DEFAULT 1,
    brand_kit     TEXT     NOT NULL DEFAULT '{}', -- JSON, versioned by its "v"
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
);

CREATE TABLE creative_renders (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER  NOT NULL REFERENCES creative_projects (project_id),
    code            TEXT     NOT NULL, -- R01, R02
    pos             INTEGER  NOT NULL,
    label           TEXT     NOT NULL DEFAULT '',
    hidden          INTEGER  NOT NULL DEFAULT 0,
    file_id         INTEGER  NOT NULL REFERENCES files (id),
    preview_file_id INTEGER  NOT NULL REFERENCES files (id),
    created_at      DATETIME NOT NULL,
    UNIQUE (project_id, code)
);

CREATE TABLE creative_concepts (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER  NOT NULL REFERENCES creative_projects (project_id),
    code         TEXT     NOT NULL, -- C1, O2, T1AR
    pos          INTEGER  NOT NULL,
    name         TEXT     NOT NULL,
    why          TEXT     NOT NULL DEFAULT '',
    primary_text TEXT     NOT NULL DEFAULT '',
    headline     TEXT     NOT NULL DEFAULT '',
    description  TEXT     NOT NULL DEFAULT '',
    cta          TEXT     NOT NULL DEFAULT '',
    copy         TEXT     NOT NULL DEFAULT '{}', -- JSON: EN and AR blocks
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL,
    UNIQUE (project_id, code)
);

CREATE TABLE creative_ads (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER  NOT NULL REFERENCES creative_projects (project_id),
    concept_id  INTEGER  NOT NULL REFERENCES creative_concepts (id),
    code        TEXT     NOT NULL, -- C1_CASH_EN_9x16_CTA
    file_name   TEXT     NOT NULL, -- MARLIN_C1_CASH_EN_9x16_CTA.jpg
    pos         INTEGER  NOT NULL,
    fmt         TEXT     NOT NULL CHECK (fmt IN ('9:16', '4:5', '1:1', '1.91:1')),
    lang        TEXT     NOT NULL CHECK (lang IN ('en', 'ar')),
    has_cta     INTEGER  NOT NULL,
    render_code TEXT     NOT NULL DEFAULT '',
    -- The layers being edited. Saving a draft doesn't change what reviewers
    -- see; publishing makes a version.
    draft       TEXT, -- JSON design file
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    UNIQUE (project_id, code)
);
CREATE INDEX creative_ads_concept ON creative_ads (concept_id, pos);

CREATE TABLE creative_ad_versions (
    id              INTEGER PRIMARY KEY,
    ad_id           INTEGER  NOT NULL REFERENCES creative_ads (id),
    version         INTEGER  NOT NULL,
    file_id         INTEGER  NOT NULL REFERENCES files (id),
    preview_file_id INTEGER  NOT NULL REFERENCES files (id),
    design          TEXT, -- JSON design file it was drawn from; NULL for an uploaded picture
    source          TEXT     NOT NULL CHECK (source IN ('upload', 'editor', 'generated')),
    created_by      INTEGER  REFERENCES users (id),
    created_at      DATETIME NOT NULL,
    UNIQUE (ad_id, version)
);

-- Reviewers type their name; it isn't verified. The latest decision per
-- version and name counts, and any "revise" outranks approvals.
CREATE TABLE creative_decisions (
    id         INTEGER PRIMARY KEY,
    version_id INTEGER  NOT NULL REFERENCES creative_ad_versions (id),
    name       TEXT     NOT NULL,
    status     TEXT     NOT NULL CHECK (status IN ('approved', 'revise', 'clear')),
    user_id    INTEGER  REFERENCES users (id),
    created_at DATETIME NOT NULL
);
CREATE INDEX creative_decisions_version ON creative_decisions (version_id, created_at);

-- Comments stay with the ad across versions and record the version they
-- were left on. A pin is a spot on the picture, as fractions of its size.
CREATE TABLE creative_comments (
    id          INTEGER PRIMARY KEY,
    ad_id       INTEGER  NOT NULL REFERENCES creative_ads (id),
    version_id  INTEGER  NOT NULL REFERENCES creative_ad_versions (id),
    name        TEXT     NOT NULL,
    body        TEXT     NOT NULL,
    pin_x       REAL CHECK (pin_x IS NULL OR (pin_x >= 0 AND pin_x <= 1)),
    pin_y       REAL CHECK (pin_y IS NULL OR (pin_y >= 0 AND pin_y <= 1)),
    user_id     INTEGER  REFERENCES users (id),
    resolved_at DATETIME,
    resolved_by INTEGER  REFERENCES users (id),
    created_at  DATETIME NOT NULL
);
CREATE INDEX creative_comments_ad ON creative_comments (ad_id, created_at);

-- A reviewer choosing which render a concept should be rebuilt on. The
-- latest pick per concept counts; an empty render clears it.
CREATE TABLE creative_render_picks (
    id          INTEGER PRIMARY KEY,
    concept_id  INTEGER  NOT NULL REFERENCES creative_concepts (id),
    render_code TEXT     NOT NULL,
    name        TEXT     NOT NULL,
    created_at  DATETIME NOT NULL
);
CREATE INDEX creative_render_picks_concept ON creative_render_picks (concept_id, created_at);

-- +goose Down
DROP TABLE creative_render_picks;
DROP TABLE creative_comments;
DROP TABLE creative_decisions;
DROP TABLE creative_ad_versions;
DROP TABLE creative_ads;
DROP TABLE creative_concepts;
DROP TABLE creative_renders;
DROP TABLE creative_projects;
DROP TABLE files;
