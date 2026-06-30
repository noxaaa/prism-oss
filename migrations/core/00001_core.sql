-- +goose Up
CREATE TABLE organizations (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  owner_user_id text REFERENCES "user"(id),
  default_rule_limit integer NOT NULL DEFAULT 0,
  default_traffic_limit_bytes bigint NOT NULL DEFAULT 0,
  default_traffic_limit_mode text NOT NULL DEFAULT 'TOTAL',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  CHECK (default_rule_limit >= 0),
  CHECK (default_traffic_limit_bytes >= 0),
  CHECK (default_traffic_limit_mode IN ('TOTAL', 'UPLOAD_ONLY', 'DOWNLOAD_ONLY', 'MAX_OF_UP_DOWN'))
);

CREATE TABLE node_groups (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id)
);

CREATE TABLE nodes (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  status text NOT NULL,
  public_description text NOT NULL DEFAULT '',
  desired_config_version integer NOT NULL DEFAULT 0,
  applied_config_version integer NOT NULL DEFAULT 0,
  config_status text NOT NULL DEFAULT 'PENDING',
  config_error_message text NOT NULL DEFAULT '',
  config_status_updated_at timestamptz,
  last_seen_at timestamptz,
  registered_at timestamptz,
  agent_version text NOT NULL DEFAULT '',
  agent_commit text NOT NULL DEFAULT '',
  agent_build_time text NOT NULL DEFAULT '',
  agent_auto_update_enabled boolean NOT NULL DEFAULT true,
  desired_agent_version text NOT NULL DEFAULT '',
  agent_update_status text NOT NULL DEFAULT 'IDLE',
  agent_update_error text NOT NULL DEFAULT '',
  agent_update_started_at timestamptz,
  agent_update_finished_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (status IN ('PENDING', 'ONLINE', 'OFFLINE', 'DISABLED')),
  CHECK (agent_update_status IN ('IDLE', 'PENDING', 'RUNNING', 'SUCCEEDED', 'FAILED'))
);

CREATE TABLE node_group_members (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id uuid NOT NULL,
  node_group_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (node_id, node_group_id),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, node_group_id) REFERENCES node_groups(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE node_listen_ips (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id uuid NOT NULL,
  listen_ip text NOT NULL,
  display_name text NOT NULL DEFAULT '',
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, node_id, id),
  UNIQUE (node_id, listen_ip),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE node_port_ranges (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id uuid NOT NULL,
  protocol text NOT NULL,
  start_port integer NOT NULL,
  end_port integer NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (protocol IN ('TCP', 'UDP')),
  CHECK (start_port BETWEEN 1 AND 65535),
  CHECK (end_port BETWEEN 1 AND 65535),
  CHECK (start_port <= end_port),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE targets (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  host text NOT NULL,
  port integer NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (port BETWEEN 1 AND 65535)
);

CREATE TABLE target_groups (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  scheduler text NOT NULL DEFAULT 'PRIORITY_IPHASH',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (length(trim(scheduler)) > 0)
);

CREATE TABLE target_group_members (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  target_group_id uuid NOT NULL,
  target_id uuid NOT NULL,
  priority integer NOT NULL,
  weight integer NOT NULL DEFAULT 1,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (target_group_id, target_id),
  FOREIGN KEY (organization_id, target_group_id) REFERENCES target_groups(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id) ON DELETE CASCADE,
  CHECK (weight >= 0)
);

CREATE TABLE inbound_bindings (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_group_id uuid NOT NULL,
  listen_ip text NOT NULL,
  protocol text NOT NULL,
  port integer NOT NULL,
  match_type text NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, node_group_id, listen_ip, protocol, port, match_type),
  CHECK (protocol IN ('TCP', 'UDP', 'TCP_UDP')),
  CHECK (port BETWEEN 1 AND 65535),
  CHECK (length(trim(match_type)) > 0),
  CHECK (match_type != 'TLS_SNI' OR protocol = 'TCP'),
  CHECK (length(trim(listen_ip)) > 0),
  FOREIGN KEY (organization_id, node_group_id) REFERENCES node_groups(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE forwarding_rules (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  owner_user_id text NOT NULL REFERENCES "user"(id),
  name text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'ENABLED',
  forwarding_type text NOT NULL DEFAULT 'DIRECT',
  protocol text NOT NULL,
  match_type text NOT NULL,
  inbound_binding_id uuid NOT NULL,
  sni_hostname text,
  target_type text NOT NULL DEFAULT 'TARGET',
  target_id uuid,
  target_group_id uuid,
  proxy_protocol_in text NOT NULL DEFAULT 'NONE',
  proxy_protocol_out text NOT NULL DEFAULT 'NONE',
  config_version integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (status IN ('ENABLED', 'DISABLED', 'OVER_LIMIT_DISABLED')),
  CHECK (protocol IN ('TCP', 'UDP', 'TCP_UDP')),
  CHECK (length(trim(forwarding_type)) > 0),
  CHECK (length(trim(match_type)) > 0),
  CHECK (match_type != 'TLS_SNI' OR (protocol = 'TCP' AND sni_hostname IS NOT NULL AND length(trim(sni_hostname)) > 0)),
  CHECK (match_type != 'ANY_INBOUND' OR sni_hostname IS NULL),
  CHECK (protocol != 'UDP' OR (match_type = 'ANY_INBOUND' AND sni_hostname IS NULL AND proxy_protocol_in = 'NONE' AND proxy_protocol_out = 'NONE')),
  CHECK (target_type IN ('TARGET', 'TARGET_GROUP')),
  CHECK ((target_type = 'TARGET' AND target_id IS NOT NULL AND target_group_id IS NULL) OR (target_type = 'TARGET_GROUP' AND target_id IS NULL AND target_group_id IS NOT NULL)),
  CHECK (proxy_protocol_in IN ('NONE', 'V1', 'V2')),
  CHECK (proxy_protocol_out IN ('NONE', 'V1', 'V2')),
  FOREIGN KEY (organization_id, inbound_binding_id) REFERENCES inbound_bindings(organization_id, id),
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id),
  FOREIGN KEY (organization_id, target_group_id) REFERENCES target_groups(organization_id, id)
);

CREATE TABLE quotas (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scope text NOT NULL,
  subject_user_id text,
  subject_rule_id uuid,
  rule_limit integer NOT NULL DEFAULT 0,
  traffic_limit_bytes bigint NOT NULL DEFAULT 0,
  traffic_limit_mode text NOT NULL,
  over_limit_action text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  CHECK (scope IN ('ORGANIZATION', 'USER', 'RULE')),
  CHECK (rule_limit >= 0),
  CHECK (traffic_limit_bytes >= 0),
  CHECK (traffic_limit_mode IN ('TOTAL', 'UPLOAD_ONLY', 'DOWNLOAD_ONLY', 'MAX_OF_UP_DOWN')),
  CHECK (over_limit_action IN ('DISABLE_RULE', 'WARN_ONLY')),
  CHECK ((scope = 'ORGANIZATION' AND subject_user_id IS NULL AND subject_rule_id IS NULL) OR (scope = 'USER' AND subject_user_id IS NOT NULL AND subject_rule_id IS NULL) OR (scope = 'RULE' AND subject_user_id IS NULL AND subject_rule_id IS NOT NULL)),
  FOREIGN KEY (subject_user_id) REFERENCES "user"(id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, subject_rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE agent_registration_tokens (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_type text NOT NULL,
  agent_id uuid NOT NULL,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  revoked_at timestamptz,
  created_by_user_id text REFERENCES "user"(id),
  created_at timestamptz NOT NULL,
  CHECK (agent_type IN ('NODE', 'MONITOR'))
);

CREATE TABLE agent_credentials (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_type text NOT NULL,
  agent_id uuid NOT NULL,
  credential_hash text NOT NULL UNIQUE,
  registration_token_id uuid,
  activated_at timestamptz,
  revoked_at timestamptz,
  rotated_at timestamptz,
  created_at timestamptz NOT NULL,
  CHECK (agent_type IN ('NODE', 'MONITOR'))
);

CREATE UNIQUE INDEX agent_credentials_pending_registration_token_unique
  ON agent_credentials(registration_token_id)
  WHERE registration_token_id IS NOT NULL AND activated_at IS NULL AND revoked_at IS NULL;

CREATE TABLE rule_tags (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  rule_id uuid NOT NULL,
  tag text NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (rule_id, tag),
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE rule_traffic_counters (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  rule_id uuid NOT NULL,
  period_start timestamptz NOT NULL,
  period_granularity text NOT NULL,
  upload_bytes bigint NOT NULL DEFAULT 0,
  download_bytes bigint NOT NULL DEFAULT 0,
  tcp_connections bigint NOT NULL DEFAULT 0,
  udp_packets bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL,
  UNIQUE (rule_id, period_start, period_granularity),
  CHECK (period_granularity IN ('HOUR', 'DAY', 'MONTH', 'ALL_TIME')),
  CHECK (upload_bytes >= 0),
  CHECK (download_bytes >= 0),
  CHECK (tcp_connections >= 0),
  CHECK (udp_packets >= 0),
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE audit_logs (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  actor_user_id text REFERENCES "user"(id),
  actor_roles_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  actor_permissions_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  action text NOT NULL,
  resource_type text NOT NULL,
  resource_id text NOT NULL,
  result text NOT NULL,
  error_message text,
  metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  source_ip text,
  created_at timestamptz NOT NULL,
  CHECK (result IN ('SUCCESS', 'FAILURE'))
);

CREATE INDEX idx_audit_logs_org_created ON audit_logs(organization_id, created_at);
CREATE INDEX idx_forwarding_rules_org_owner ON forwarding_rules(organization_id, owner_user_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_agent_registration_token_agent()
RETURNS trigger AS $$
BEGIN
  IF NEW.agent_type = 'NODE' AND NOT EXISTS (
    SELECT 1
    FROM nodes
    WHERE nodes.organization_id = NEW.organization_id
      AND nodes.id = NEW.agent_id
      AND nodes.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION 'NODE registration token must reference a same-organization node';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER validate_agent_registration_tokens_agent_insert
  BEFORE INSERT ON agent_registration_tokens
  FOR EACH ROW EXECUTE FUNCTION validate_agent_registration_token_agent();

CREATE TRIGGER validate_agent_registration_tokens_agent_update
  BEFORE UPDATE OF organization_id, agent_type, agent_id ON agent_registration_tokens
  FOR EACH ROW EXECUTE FUNCTION validate_agent_registration_token_agent();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_agent_credential_agent()
RETURNS trigger AS $$
BEGIN
  IF NEW.agent_type = 'NODE' AND NOT EXISTS (
    SELECT 1
    FROM nodes
    WHERE nodes.organization_id = NEW.organization_id
      AND nodes.id = NEW.agent_id
      AND nodes.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION 'NODE credential must reference a same-organization node';
  END IF;
  IF NEW.registration_token_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
    FROM agent_registration_tokens
    WHERE agent_registration_tokens.organization_id = NEW.organization_id
      AND agent_registration_tokens.agent_type = NEW.agent_type
      AND agent_registration_tokens.agent_id = NEW.agent_id
      AND agent_registration_tokens.id = NEW.registration_token_id
  ) THEN
    RAISE EXCEPTION 'agent credential registration token must reference the same agent';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER validate_agent_credentials_agent_insert
  BEFORE INSERT ON agent_credentials
  FOR EACH ROW EXECUTE FUNCTION validate_agent_credential_agent();

CREATE TRIGGER validate_agent_credentials_agent_update
  BEFORE UPDATE OF organization_id, agent_type, agent_id, registration_token_id ON agent_credentials
  FOR EACH ROW EXECUTE FUNCTION validate_agent_credential_agent();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION revoke_node_agent_auth()
RETURNS trigger AS $$
DECLARE
  revoke_time timestamptz;
BEGIN
  IF TG_OP = 'DELETE' THEN
    revoke_time := clock_timestamp();
  ELSE
    revoke_time := COALESCE(NEW.deleted_at, clock_timestamp());
  END IF;
  UPDATE agent_registration_tokens
  SET revoked_at = COALESCE(revoked_at, revoke_time)
  WHERE organization_id = OLD.organization_id
    AND agent_type = 'NODE'
    AND agent_id = OLD.id;
  UPDATE agent_credentials
  SET revoked_at = COALESCE(revoked_at, revoke_time)
  WHERE organization_id = OLD.organization_id
    AND agent_type = 'NODE'
    AND agent_id = OLD.id;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER revoke_node_agent_auth_on_delete
  AFTER DELETE ON nodes
  FOR EACH ROW EXECUTE FUNCTION revoke_node_agent_auth();

CREATE TRIGGER revoke_node_agent_auth_on_soft_delete
  AFTER UPDATE OF deleted_at ON nodes
  FOR EACH ROW
  WHEN (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL)
  EXECUTE FUNCTION revoke_node_agent_auth();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_inbound_binding_rule_compatibility()
RETURNS trigger AS $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM forwarding_rules
    WHERE forwarding_rules.organization_id = OLD.organization_id
      AND forwarding_rules.inbound_binding_id = OLD.id
      AND forwarding_rules.deleted_at IS NULL
      AND (
        forwarding_rules.organization_id != NEW.organization_id
        OR forwarding_rules.protocol != NEW.protocol
        OR forwarding_rules.match_type != NEW.match_type
      )
  ) THEN
    RAISE EXCEPTION 'inbound binding update would make forwarding rules incompatible';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER validate_inbound_binding_rule_compatibility_update
  BEFORE UPDATE OF organization_id, protocol, match_type ON inbound_bindings
  FOR EACH ROW EXECUTE FUNCTION validate_inbound_binding_rule_compatibility();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_forwarding_rule()
RETURNS trigger AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM inbound_bindings
    WHERE inbound_bindings.organization_id = NEW.organization_id
      AND inbound_bindings.id = NEW.inbound_binding_id
      AND inbound_bindings.protocol = NEW.protocol
      AND inbound_bindings.match_type = NEW.match_type
  ) THEN
    RAISE EXCEPTION 'forwarding rule inbound binding must match protocol and match type';
  END IF;

  IF NEW.deleted_at IS NULL AND NEW.enabled AND NEW.status = 'ENABLED' THEN
    IF NEW.match_type = 'ANY_INBOUND' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'ANY_INBOUND rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type = 'TLS_SNI' AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
        AND (
          existing_rule.match_type != 'TLS_SNI'
          OR btrim(existing_binding.listen_ip, '[]') != btrim(new_binding.listen_ip, '[]')
          OR lower(existing_rule.sni_hostname) = lower(NEW.sni_hostname)
        )
    ) THEN
      RAISE EXCEPTION 'TLS_SNI rule already exists on this inbound endpoint';
    END IF;

    IF NEW.match_type NOT IN ('ANY_INBOUND', 'TLS_SNI') AND EXISTS (
      SELECT 1
      FROM forwarding_rules AS existing_rule
      JOIN inbound_bindings AS existing_binding
        ON existing_binding.organization_id = existing_rule.organization_id
       AND existing_binding.id = existing_rule.inbound_binding_id
      JOIN inbound_bindings AS new_binding
        ON new_binding.organization_id = NEW.organization_id
       AND new_binding.id = NEW.inbound_binding_id
      WHERE existing_rule.id != NEW.id
        AND existing_rule.organization_id = NEW.organization_id
        AND existing_rule.enabled
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR btrim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR btrim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    ) THEN
      RAISE EXCEPTION 'unsupported match type conflicts with existing inbound endpoint';
    END IF;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER validate_forwarding_rules_inbound_insert
  BEFORE INSERT ON forwarding_rules
  FOR EACH ROW EXECUTE FUNCTION validate_forwarding_rule();

CREATE TRIGGER validate_forwarding_rules_inbound_update
  BEFORE UPDATE OF organization_id, enabled, status, protocol, match_type, inbound_binding_id, sni_hostname, target_type, target_id, target_group_id, deleted_at ON forwarding_rules
  FOR EACH ROW EXECUTE FUNCTION validate_forwarding_rule();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prevent_audit_logs_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit logs are append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER prevent_audit_logs_update
  BEFORE UPDATE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION prevent_audit_logs_mutation();

CREATE TRIGGER prevent_audit_logs_delete
  BEFORE DELETE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION prevent_audit_logs_mutation();

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS rule_traffic_counters;
DROP TABLE IF EXISTS rule_tags;
DROP TABLE IF EXISTS agent_credentials;
DROP TABLE IF EXISTS agent_registration_tokens;
DROP TABLE IF EXISTS quotas;
DROP TABLE IF EXISTS forwarding_rules;
DROP TABLE IF EXISTS inbound_bindings;
DROP TABLE IF EXISTS target_group_members;
DROP TABLE IF EXISTS target_groups;
DROP TABLE IF EXISTS targets;
DROP TABLE IF EXISTS node_port_ranges;
DROP TABLE IF EXISTS node_listen_ips;
DROP TABLE IF EXISTS node_group_members;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS node_groups;
DROP TABLE IF EXISTS organizations;
DROP FUNCTION IF EXISTS prevent_audit_logs_mutation();
DROP FUNCTION IF EXISTS validate_forwarding_rule();
DROP FUNCTION IF EXISTS validate_inbound_binding_rule_compatibility();
DROP FUNCTION IF EXISTS revoke_node_agent_auth();
DROP FUNCTION IF EXISTS validate_agent_credential_agent();
DROP FUNCTION IF EXISTS validate_agent_registration_token_agent();
