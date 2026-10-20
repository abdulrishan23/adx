-- +goose Up
-- A campaign is one push for a project: an offer, a window, a budget and
-- channels. Ads, pages and the plan made for that push belong to it, so one
-- screen can say what's done and what's next (BD-64).
CREATE TABLE campaigns (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    project_id   INTEGER  NOT NULL REFERENCES projects (id),
    name         TEXT     NOT NULL COLLATE NOCASE,
    goal         TEXT     NOT NULL DEFAULT '',
    audience     TEXT     NOT NULL DEFAULT '',
    avoid        TEXT     NOT NULL DEFAULT '',
    offer_id     INTEGER  REFERENCES offers (id),
    starts_on    DATE,
    ends_on      DATE,
    budget_minor INTEGER,
    currency     TEXT     NOT NULL DEFAULT 'AED',
    channels     TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(channels)),
    markets      TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(markets)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL,
    UNIQUE (project_id, name)
);
CREATE INDEX campaigns_project ON campaigns (workspace_id, project_id);

ALTER TABLE creative_concepts ADD COLUMN campaign_id INTEGER REFERENCES campaigns (id);
ALTER TABLE pages ADD COLUMN campaign_id INTEGER REFERENCES campaigns (id);
ALTER TABLE plans ADD COLUMN campaign_id INTEGER REFERENCES campaigns (id);

-- +goose Down
ALTER TABLE plans DROP COLUMN campaign_id;
ALTER TABLE pages DROP COLUMN campaign_id;
ALTER TABLE creative_concepts DROP COLUMN campaign_id;
DROP TABLE campaigns;
