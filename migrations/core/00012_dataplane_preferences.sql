-- +goose Up
ALTER TABLE nodes
  ADD COLUMN dataplane_mode text NOT NULL DEFAULT 'AUTO',
  ADD COLUMN dataplane_conflict_policy text NOT NULL DEFAULT 'FAIL_FAST',
  ADD COLUMN dataplane_instance_id text NOT NULL DEFAULT '',
  ADD COLUMN dataplane_status text NOT NULL DEFAULT 'UNKNOWN',
  ADD COLUMN dataplane_error text NOT NULL DEFAULT '',
  ADD COLUMN dataplane_last_hash text NOT NULL DEFAULT '',
  ADD COLUMN dataplane_last_applied_at timestamptz,
  ADD CONSTRAINT nodes_dataplane_mode_check
    CHECK (dataplane_mode IN ('AUTO', 'NATIVE', 'HAPROXY', 'NFTABLES')),
  ADD CONSTRAINT nodes_dataplane_conflict_policy_check
    CHECK (dataplane_conflict_policy IN ('FAIL_FAST'));

ALTER TABLE forwarding_rules
  ADD COLUMN dataplane_preference text NOT NULL DEFAULT 'AUTO',
  ADD CONSTRAINT forwarding_rules_dataplane_preference_check
    CHECK (dataplane_preference IN ('AUTO', 'NATIVE', 'HAPROXY', 'NFTABLES'));

ALTER TABLE rule_deployment_statuses
  ADD COLUMN expected_dataplane text NOT NULL DEFAULT '',
  ADD COLUMN actual_dataplane text NOT NULL DEFAULT '',
  ADD COLUMN owner text NOT NULL DEFAULT '',
  ADD COLUMN drift_status text NOT NULL DEFAULT '',
  ADD COLUMN external_resource text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE rule_deployment_statuses
  DROP COLUMN IF EXISTS external_resource,
  DROP COLUMN IF EXISTS drift_status,
  DROP COLUMN IF EXISTS owner,
  DROP COLUMN IF EXISTS actual_dataplane,
  DROP COLUMN IF EXISTS expected_dataplane;

ALTER TABLE forwarding_rules
  DROP CONSTRAINT IF EXISTS forwarding_rules_dataplane_preference_check,
  DROP COLUMN IF EXISTS dataplane_preference;

ALTER TABLE nodes
  DROP CONSTRAINT IF EXISTS nodes_dataplane_conflict_policy_check,
  DROP CONSTRAINT IF EXISTS nodes_dataplane_mode_check,
  DROP COLUMN IF EXISTS dataplane_last_applied_at,
  DROP COLUMN IF EXISTS dataplane_last_hash,
  DROP COLUMN IF EXISTS dataplane_error,
  DROP COLUMN IF EXISTS dataplane_status,
  DROP COLUMN IF EXISTS dataplane_instance_id,
  DROP COLUMN IF EXISTS dataplane_conflict_policy,
  DROP COLUMN IF EXISTS dataplane_mode;
