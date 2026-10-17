-- +goose Up
-- Google's OAuth uses PKCE; the verifier is kept with the attempt so the
-- callback can send it. GoHighLevel attempts leave it empty.
ALTER TABLE connection_oauth_attempts ADD COLUMN code_verifier TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE connection_oauth_attempts DROP COLUMN code_verifier;
