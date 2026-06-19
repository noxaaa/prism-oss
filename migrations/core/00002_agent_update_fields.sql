-- +goose Up
ALTER TABLE nodes ADD COLUMN agent_version TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_build_time TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_auto_update_enabled INTEGER NOT NULL DEFAULT 1 CHECK(agent_auto_update_enabled IN (0, 1));
ALTER TABLE nodes ADD COLUMN desired_agent_version TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_update_status TEXT NOT NULL DEFAULT 'IDLE' CHECK(agent_update_status IN ('IDLE', 'PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED'));
ALTER TABLE nodes ADD COLUMN agent_update_error TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_update_started_at TEXT;
ALTER TABLE nodes ADD COLUMN agent_update_finished_at TEXT;

-- +goose Down
ALTER TABLE nodes DROP COLUMN agent_update_finished_at;
ALTER TABLE nodes DROP COLUMN agent_update_started_at;
ALTER TABLE nodes DROP COLUMN agent_update_error;
ALTER TABLE nodes DROP COLUMN agent_update_status;
ALTER TABLE nodes DROP COLUMN desired_agent_version;
ALTER TABLE nodes DROP COLUMN agent_auto_update_enabled;
ALTER TABLE nodes DROP COLUMN agent_build_time;
ALTER TABLE nodes DROP COLUMN agent_commit;
ALTER TABLE nodes DROP COLUMN agent_version;
