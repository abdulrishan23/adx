-- +goose Up
-- An alert card in the Inbox can be snoozed or dismissed. Snoozed keeps the
-- alert open and hides it from the fronts until the date; the check firing
-- again meanwhile adds an occurrence without surfacing it. Dismissed ends
-- the alert (state resolved) with the reason kept; the same check firing on
-- a later day opens a new alert.
ALTER TABLE alerts ADD COLUMN snoozed_until DATETIME;
ALTER TABLE alerts ADD COLUMN dismiss_reason TEXT;

-- +goose Down
ALTER TABLE alerts DROP COLUMN dismiss_reason;
ALTER TABLE alerts DROP COLUMN snoozed_until;
