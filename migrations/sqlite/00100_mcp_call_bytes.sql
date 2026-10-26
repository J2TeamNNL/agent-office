-- +goose Up
-- ADR-183: what a tool call through the gateway weighed, to measure what
-- cutting long results saves: the request, the result the agent got, and
-- the bytes office cut out of it (kept for mcp_result).
ALTER TABLE mcp_calls ADD COLUMN bytes_in INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mcp_calls ADD COLUMN bytes_out INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mcp_calls ADD COLUMN bytes_cut INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE mcp_calls DROP COLUMN bytes_cut;
ALTER TABLE mcp_calls DROP COLUMN bytes_out;
ALTER TABLE mcp_calls DROP COLUMN bytes_in;
