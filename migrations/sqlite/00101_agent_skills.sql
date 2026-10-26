-- +goose Up
-- ADR-185: the skills an agent keeps at hand. The prompt lists their names and
-- why (the instructions are read with use_skill when a task calls for one);
-- uses and last_used_at show which ones earn their place.
CREATE TABLE agent_skills (
    agent_id     TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    why          TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'person', -- person | agent | setup | library | call
    uses         INTEGER NOT NULL DEFAULT 0,
    last_used_at TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    PRIMARY KEY (agent_id, name)
);

-- +goose Down
DROP TABLE agent_skills;
