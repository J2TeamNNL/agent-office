-- +goose Up
-- ADR-197: who follows a piece of the work board up (reads its result,
-- accepts or rewrites it): an agent key of the project; '' = its default agent.
ALTER TABLE burn_items ADD COLUMN owner TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE burn_items DROP COLUMN owner;
