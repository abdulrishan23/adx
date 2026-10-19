-- +goose Up
-- A person's choice for a plan line that matched more than one running
-- campaign (BD-60: ADX never updates both unasked). Keyed by the plan
-- version and the line's position in it, which an immutable version keeps
-- stable. Recorded before any intent is written, so the choice outlives the
-- screen that asked for it.
CREATE TABLE launch_picks (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    plan_id       INTEGER  NOT NULL REFERENCES plans (id),
    plan_version  INTEGER  NOT NULL,
    line_index    INTEGER  NOT NULL,
    channel       TEXT     NOT NULL,
    campaign_id   INTEGER  NOT NULL REFERENCES ad_campaigns (id),
    picked_by     INTEGER  REFERENCES users (id),
    picked_at     DATETIME NOT NULL,
    UNIQUE (plan_id, plan_version, line_index)
);

-- +goose Down
DROP TABLE launch_picks;
