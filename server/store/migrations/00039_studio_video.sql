-- +goose Up
-- Video ads in the studio. An ad is static (a picture in one of the four
-- sizes) or video (an MP4 in 9:16 with a poster as its preview). A version's
-- method says how it was made: upload, the editor, the generator, motion
-- from renders, or a video model.
ALTER TABLE creative_ads ADD COLUMN kind TEXT NOT NULL DEFAULT 'static' CHECK (kind IN ('static', 'video'));
ALTER TABLE creative_ad_versions ADD COLUMN method TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE creative_ad_versions DROP COLUMN method;
ALTER TABLE creative_ads DROP COLUMN kind;
