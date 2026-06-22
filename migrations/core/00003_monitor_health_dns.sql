-- +goose Up
CREATE TABLE monitor_groups (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id)
);

CREATE TABLE monitors (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  status text NOT NULL,
  desired_config_version integer NOT NULL DEFAULT 0,
  applied_config_version integer NOT NULL DEFAULT 0,
  last_seen_at timestamptz,
  registered_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (status IN ('PENDING', 'ONLINE', 'OFFLINE', 'DISABLED'))
);

CREATE TABLE monitor_group_members (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  monitor_id uuid NOT NULL,
  monitor_group_id uuid NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (monitor_id, monitor_group_id),
  FOREIGN KEY (organization_id, monitor_id) REFERENCES monitors(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, monitor_group_id) REFERENCES monitor_groups(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE health_checks (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  probe_type text NOT NULL,
  interval_seconds integer NOT NULL,
  timeout_seconds integer NOT NULL,
  config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (probe_type IN ('ICMP', 'TCP_PORT', 'HTTP')),
  CHECK (interval_seconds > 0),
  CHECK (timeout_seconds > 0)
);

CREATE TABLE health_check_targets (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  health_check_id uuid NOT NULL,
  scope_type text NOT NULL,
  target_id uuid,
  target_group_id uuid,
  created_at timestamptz NOT NULL,
  CHECK (scope_type IN ('TARGET', 'TARGET_GROUP')),
  CHECK ((scope_type = 'TARGET' AND target_id IS NOT NULL AND target_group_id IS NULL) OR (scope_type = 'TARGET_GROUP' AND target_group_id IS NOT NULL)),
  UNIQUE (organization_id, id),
  UNIQUE (organization_id, health_check_id, id),
  FOREIGN KEY (organization_id, health_check_id) REFERENCES health_checks(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, target_group_id) REFERENCES target_groups(organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uniq_health_check_targets_direct
  ON health_check_targets(health_check_id, target_id)
  WHERE scope_type = 'TARGET' AND target_id IS NOT NULL;

CREATE UNIQUE INDEX uniq_health_check_targets_group_member
  ON health_check_targets(health_check_id, target_group_id, target_id)
  WHERE scope_type = 'TARGET_GROUP' AND target_id IS NOT NULL;

CREATE UNIQUE INDEX uniq_health_check_targets_group_binding
  ON health_check_targets(health_check_id, target_group_id)
  WHERE scope_type = 'TARGET_GROUP' AND target_id IS NULL;

CREATE TABLE health_check_monitor_scopes (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  health_check_id uuid NOT NULL,
  scope_type text NOT NULL,
  monitor_id uuid,
  monitor_group_id uuid,
  created_at timestamptz NOT NULL,
  CHECK ((scope_type = 'MONITOR' AND monitor_id IS NOT NULL AND monitor_group_id IS NULL) OR (scope_type = 'MONITOR_GROUP' AND monitor_id IS NULL AND monitor_group_id IS NOT NULL)),
  FOREIGN KEY (organization_id, health_check_id) REFERENCES health_checks(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, monitor_id) REFERENCES monitors(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, monitor_group_id) REFERENCES monitor_groups(organization_id, id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uniq_health_check_monitor_scopes_monitor
  ON health_check_monitor_scopes(health_check_id, monitor_id)
  WHERE scope_type = 'MONITOR';

CREATE UNIQUE INDEX uniq_health_check_monitor_scopes_monitor_group
  ON health_check_monitor_scopes(health_check_id, monitor_group_id)
  WHERE scope_type = 'MONITOR_GROUP';

CREATE TABLE health_results (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  health_check_id uuid NOT NULL,
  health_check_target_id uuid NOT NULL,
  monitor_id uuid NOT NULL,
  target_id uuid NOT NULL,
  status text NOT NULL,
  latency_ms integer,
  error_message text NOT NULL DEFAULT '',
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  CHECK (status IN ('ONLINE', 'OFFLINE', 'UNKNOWN')),
  CHECK (latency_ms IS NULL OR latency_ms >= 0),
  FOREIGN KEY (organization_id, health_check_id) REFERENCES health_checks(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, health_check_target_id) REFERENCES health_check_targets(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, health_check_id, health_check_target_id) REFERENCES health_check_targets(organization_id, health_check_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, monitor_id) REFERENCES monitors(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE health_evaluation_rules (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  health_check_id uuid NOT NULL,
  name text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  expression_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  FOREIGN KEY (organization_id, health_check_id) REFERENCES health_checks(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE health_events (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  health_evaluation_rule_id uuid NOT NULL,
  event_type text NOT NULL,
  config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  CHECK (event_type IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE', 'WEBHOOK', 'EMAIL')),
  FOREIGN KEY (organization_id, health_evaluation_rule_id) REFERENCES health_evaluation_rules(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE dns_credentials (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  provider text NOT NULL,
  name text NOT NULL,
  encrypted_secret text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  UNIQUE (organization_id, id),
  CHECK (provider IN ('CLOUDFLARE'))
);

CREATE TABLE dns_records (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  dns_credential_id uuid NOT NULL,
  zone text NOT NULL,
  record_name text NOT NULL,
  record_type text NOT NULL,
  managed_mode text NOT NULL DEFAULT 'CUSTOMER_CREDENTIAL',
  desired_values_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_applied_values_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  last_applied_at timestamptz,
  pending_retire_dns_credential_id uuid,
  pending_retire_zone text,
  pending_retire_record_name text,
  pending_retire_record_type text,
  pending_retire_values_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  pending_retire_at timestamptz,
  provider_delete_pending_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz,
  CHECK (record_type IN ('A', 'AAAA', 'CNAME')),
  CHECK (managed_mode = 'CUSTOMER_CREDENTIAL'),
  FOREIGN KEY (organization_id, dns_credential_id) REFERENCES dns_credentials(organization_id, id),
  FOREIGN KEY (organization_id, pending_retire_dns_credential_id) REFERENCES dns_credentials(organization_id, id)
);

CREATE UNIQUE INDEX uniq_dns_records_active_name
  ON dns_records(organization_id, zone, record_name, record_type)
  WHERE deleted_at IS NULL;

CREATE INDEX idx_health_results_check_target_time ON health_results(health_check_id, target_id, observed_at);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_monitor_agent_auth()
RETURNS trigger AS $$
BEGIN
  IF NEW.agent_type = 'MONITOR' AND NOT EXISTS (
    SELECT 1
    FROM monitors
    WHERE monitors.organization_id = NEW.organization_id
      AND monitors.id = NEW.agent_id
      AND monitors.deleted_at IS NULL
  ) THEN
    IF TG_TABLE_NAME = 'agent_registration_tokens' THEN
      RAISE EXCEPTION 'MONITOR registration token must reference a same-organization monitor';
    END IF;
    RAISE EXCEPTION 'MONITOR credential must reference a same-organization monitor';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER validate_monitor_registration_tokens_agent_insert
  BEFORE INSERT ON agent_registration_tokens
  FOR EACH ROW EXECUTE FUNCTION validate_monitor_agent_auth();

CREATE TRIGGER validate_monitor_registration_tokens_agent_update
  BEFORE UPDATE OF organization_id, agent_type, agent_id ON agent_registration_tokens
  FOR EACH ROW EXECUTE FUNCTION validate_monitor_agent_auth();

CREATE TRIGGER validate_monitor_credentials_agent_insert
  BEFORE INSERT ON agent_credentials
  FOR EACH ROW EXECUTE FUNCTION validate_monitor_agent_auth();

CREATE TRIGGER validate_monitor_credentials_agent_update
  BEFORE UPDATE OF organization_id, agent_type, agent_id ON agent_credentials
  FOR EACH ROW EXECUTE FUNCTION validate_monitor_agent_auth();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION revoke_monitor_agent_auth()
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
    AND agent_type = 'MONITOR'
    AND agent_id = OLD.id;
  UPDATE agent_credentials
  SET revoked_at = COALESCE(revoked_at, revoke_time)
  WHERE organization_id = OLD.organization_id
    AND agent_type = 'MONITOR'
    AND agent_id = OLD.id;
  IF TG_OP = 'DELETE' THEN
    RETURN OLD;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER revoke_monitor_agent_auth_on_delete
  AFTER DELETE ON monitors
  FOR EACH ROW EXECUTE FUNCTION revoke_monitor_agent_auth();

CREATE TRIGGER revoke_monitor_agent_auth_on_soft_delete
  AFTER UPDATE OF deleted_at ON monitors
  FOR EACH ROW
  WHEN (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL)
  EXECUTE FUNCTION revoke_monitor_agent_auth();

-- +goose Down
DROP TRIGGER IF EXISTS revoke_monitor_agent_auth_on_soft_delete ON monitors;
DROP TRIGGER IF EXISTS revoke_monitor_agent_auth_on_delete ON monitors;
DROP TRIGGER IF EXISTS validate_monitor_credentials_agent_update ON agent_credentials;
DROP TRIGGER IF EXISTS validate_monitor_credentials_agent_insert ON agent_credentials;
DROP TRIGGER IF EXISTS validate_monitor_registration_tokens_agent_update ON agent_registration_tokens;
DROP TRIGGER IF EXISTS validate_monitor_registration_tokens_agent_insert ON agent_registration_tokens;
DROP TABLE IF EXISTS dns_records;
DROP TABLE IF EXISTS dns_credentials;
DROP TABLE IF EXISTS health_events;
DROP TABLE IF EXISTS health_evaluation_rules;
DROP TABLE IF EXISTS health_results;
DROP TABLE IF EXISTS health_check_monitor_scopes;
DROP TABLE IF EXISTS health_check_targets;
DROP TABLE IF EXISTS health_checks;
DROP TABLE IF EXISTS monitor_group_members;
DROP TABLE IF EXISTS monitors;
DROP TABLE IF EXISTS monitor_groups;
DROP FUNCTION IF EXISTS revoke_monitor_agent_auth();
DROP FUNCTION IF EXISTS validate_monitor_agent_auth();
