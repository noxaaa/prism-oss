-- +goose Up
ALTER TABLE forwarding_rules
  ADD COLUMN failure_policy text NOT NULL DEFAULT 'KEEP_ENABLED',
  ADD CONSTRAINT forwarding_rules_failure_policy_check
    CHECK (failure_policy IN ('KEEP_ENABLED', 'DISABLE_WHEN_ALL_NODES_FAILED'));

ALTER TABLE nodes
  ADD COLUMN config_status_config_version integer NOT NULL DEFAULT 0,
  ADD COLUMN config_retry_count integer NOT NULL DEFAULT 0,
  ADD COLUMN config_next_retry_at timestamptz;

CREATE TABLE rule_deployment_statuses (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  rule_id uuid NOT NULL,
  node_id uuid NOT NULL,
  config_version integer NOT NULL,
  rule_config_version integer NOT NULL,
  status text NOT NULL,
  error_code text NOT NULL DEFAULT '',
  error_message text NOT NULL DEFAULT '',
  protocol text NOT NULL DEFAULT '',
  listen_ip text NOT NULL DEFAULT '',
  port integer NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, rule_id, node_id),
  CHECK (status IN ('PENDING', 'APPLIED', 'FAILED')),
  CHECK (port >= 0 AND port <= 65535),
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX rule_deployment_statuses_org_rule_idx
  ON rule_deployment_statuses (organization_id, rule_id);

CREATE INDEX rule_deployment_statuses_org_node_idx
  ON rule_deployment_statuses (organization_id, node_id);

WITH eligible_rule_deployments AS (
  SELECT
    forwarding_rules.organization_id,
    forwarding_rules.id AS rule_id,
    nodes.id AS node_id,
    GREATEST(nodes.desired_config_version, forwarding_rules.config_version) AS config_version,
    forwarding_rules.config_version AS rule_config_version,
    CASE
      WHEN nodes.config_status = 'APPLIED'
       AND nodes.applied_config_version >= GREATEST(nodes.desired_config_version, forwarding_rules.config_version)
      THEN 'APPLIED'
      ELSE 'PENDING'
    END AS status,
    md5(
      forwarding_rules.organization_id::text || ':' ||
      forwarding_rules.id::text || ':' ||
      nodes.id::text || ':rule_deployment'
    ) AS deployment_hash
  FROM forwarding_rules
  JOIN inbound_bindings
    ON inbound_bindings.organization_id = forwarding_rules.organization_id
   AND inbound_bindings.id = forwarding_rules.inbound_binding_id
  JOIN node_group_members
    ON node_group_members.organization_id = inbound_bindings.organization_id
   AND node_group_members.node_group_id = inbound_bindings.node_group_id
  JOIN nodes
    ON nodes.organization_id = node_group_members.organization_id
   AND nodes.id = node_group_members.node_id
  LEFT JOIN target_groups
    ON target_groups.organization_id = forwarding_rules.organization_id
   AND target_groups.id = forwarding_rules.target_group_id
  WHERE forwarding_rules.deleted_at IS NULL
    AND forwarding_rules.enabled
    AND forwarding_rules.status = 'ENABLED'
    AND forwarding_rules.forwarding_type = 'DIRECT'
    AND forwarding_rules.match_type IN ('ANY_INBOUND', 'TLS_SNI')
    AND nodes.deleted_at IS NULL
    AND (
      forwarding_rules.target_type != 'TARGET_GROUP'
      OR target_groups.scheduler IN ('PRIORITY_IPHASH', 'LEAST_LOAD')
    )
)
INSERT INTO rule_deployment_statuses (
  id,
  organization_id,
  rule_id,
  node_id,
  config_version,
  rule_config_version,
  status,
  updated_at
)
SELECT
  (
    substr(deployment_hash, 1, 8) || '-' ||
    substr(deployment_hash, 9, 4) || '-' ||
    substr(deployment_hash, 13, 4) || '-' ||
    substr(deployment_hash, 17, 4) || '-' ||
    substr(deployment_hash, 21, 12)
  )::uuid,
  organization_id,
  rule_id,
  node_id,
  config_version,
  rule_config_version,
  status,
  clock_timestamp()
FROM eligible_rule_deployments
ON CONFLICT (organization_id, rule_id, node_id) DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS rule_deployment_statuses_org_node_idx;
DROP INDEX IF EXISTS rule_deployment_statuses_org_rule_idx;
DROP TABLE IF EXISTS rule_deployment_statuses;

ALTER TABLE nodes
  DROP COLUMN IF EXISTS config_next_retry_at,
  DROP COLUMN IF EXISTS config_retry_count,
  DROP COLUMN IF EXISTS config_status_config_version;

ALTER TABLE forwarding_rules
  DROP CONSTRAINT IF EXISTS forwarding_rules_failure_policy_check,
  DROP COLUMN IF EXISTS failure_policy;
