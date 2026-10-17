-- +goose Up
-- Where a file came from. A template render is a template render; nothing
-- ADX produces today is AI-generated, and the column exists so that if a
-- generator is ever wired its output is labelled, never mistaken for a
-- photo or a layout.
ALTER TABLE assets ADD COLUMN provenance TEXT NOT NULL DEFAULT 'uploaded' CHECK (provenance IN ('uploaded', 'template_render', 'ai_generated'));

-- A render is a durable run; the version remembers which run produced its
-- output and the last safe error when one failed. bindings records the
-- offer version and approved facts the copy was checked against when it was
-- rendered, so a later change to either can be seen against this version.
ALTER TABLE creative_versions ADD COLUMN render_run_id INTEGER REFERENCES runs (id);
ALTER TABLE creative_versions ADD COLUMN render_error TEXT NOT NULL DEFAULT '';
ALTER TABLE creative_versions ADD COLUMN bindings TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(bindings));

-- A creative may be built for a plan version's asset requirements.
ALTER TABLE creatives ADD COLUMN plan_id INTEGER REFERENCES plans (id);
ALTER TABLE creatives ADD COLUMN plan_version INTEGER;
CREATE INDEX creatives_plan ON creatives (plan_id);

-- +goose Down
DROP INDEX creatives_plan;
ALTER TABLE creatives DROP COLUMN plan_version;
ALTER TABLE creatives DROP COLUMN plan_id;
ALTER TABLE creative_versions DROP COLUMN bindings;
ALTER TABLE creative_versions DROP COLUMN render_error;
ALTER TABLE creative_versions DROP COLUMN render_run_id;
ALTER TABLE assets DROP COLUMN provenance;
