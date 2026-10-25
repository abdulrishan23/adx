-- +goose Up
-- Which of the accounts a credential reaches ADX actually reads (D-26). NULL
-- means on. A Google or Meta sign-in can reach accounts that aren't the
-- client's, so discovered ad accounts start off and an admin switches on the
-- ones that belong. GoHighLevel locations are connected one at a time on
-- purpose, so they start on. Everything connected before this column existed
-- stays on: it was already being read.
ALTER TABLE accounts ADD COLUMN disabled_at DATETIME;

-- +goose Down
ALTER TABLE accounts DROP COLUMN disabled_at;
