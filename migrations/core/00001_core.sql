-- +goose Up
-- BetterAuth-owned auth tables. This block is generated from better-auth@1.6.16
-- through better-auth/db/migration for the configured SQLite adapter.
CREATE TABLE "user" ("id" text not null primary key, "name" text not null, "email" text not null unique, "emailVerified" integer not null, "image" text, "createdAt" date not null, "updatedAt" date not null);
CREATE TABLE "session" ("id" text not null primary key, "expiresAt" date not null, "token" text not null unique, "createdAt" date not null, "updatedAt" date not null, "ipAddress" text, "userAgent" text, "userId" text not null references "user" ("id") on delete cascade);
CREATE TABLE "account" ("id" text not null primary key, "accountId" text not null, "providerId" text not null, "userId" text not null references "user" ("id") on delete cascade, "accessToken" text, "refreshToken" text, "idToken" text, "accessTokenExpiresAt" date, "refreshTokenExpiresAt" date, "scope" text, "password" text, "createdAt" date not null, "updatedAt" date not null);
CREATE TABLE "verification" ("id" text not null primary key, "identifier" text not null, "value" text not null, "expiresAt" date not null, "createdAt" date not null, "updatedAt" date not null);
CREATE INDEX "session_userId_idx" on "session" ("userId");
CREATE INDEX "account_userId_idx" on "account" ("userId");
CREATE INDEX "verification_identifier_idx" on "verification" ("identifier");
CREATE TABLE organizations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  owner_user_id TEXT REFERENCES "user"(id),
  default_rule_limit INTEGER NOT NULL DEFAULT 0,
  default_traffic_limit_bytes INTEGER NOT NULL DEFAULT 0,
  default_traffic_limit_mode TEXT NOT NULL DEFAULT 'TOTAL',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  CHECK(default_rule_limit >= 0),
  CHECK(default_traffic_limit_bytes >= 0),
  CHECK(default_traffic_limit_mode IN ('TOTAL', 'UPLOAD_ONLY', 'DOWNLOAD_ONLY', 'MAX_OF_UP_DOWN'))
);
CREATE TABLE quotas (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scope TEXT NOT NULL,
  subject_user_id TEXT,
  subject_rule_id TEXT,
  rule_limit INTEGER NOT NULL DEFAULT 0,
  traffic_limit_bytes INTEGER NOT NULL DEFAULT 0,
  traffic_limit_mode TEXT NOT NULL,
  over_limit_action TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK(scope IN ('ORGANIZATION', 'USER', 'RULE')),
  CHECK(rule_limit >= 0),
  CHECK(traffic_limit_bytes >= 0),
  CHECK(traffic_limit_mode IN ('TOTAL', 'UPLOAD_ONLY', 'DOWNLOAD_ONLY', 'MAX_OF_UP_DOWN')),
  CHECK(over_limit_action IN ('DISABLE_RULE', 'WARN_ONLY')),
  CHECK( (scope = 'ORGANIZATION' AND subject_user_id IS NULL AND subject_rule_id IS NULL) OR (scope = 'USER' AND subject_user_id IS NOT NULL AND subject_rule_id IS NULL) OR (scope = 'RULE' AND subject_user_id IS NULL AND subject_rule_id IS NOT NULL) ),
  FOREIGN KEY (subject_user_id) REFERENCES "user"(id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, subject_rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE node_groups (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(organization_id, id)
);
CREATE TABLE nodes (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  public_description TEXT NOT NULL DEFAULT '',
  desired_config_version INTEGER NOT NULL DEFAULT 0,
  applied_config_version INTEGER NOT NULL DEFAULT 0,
  config_status TEXT NOT NULL DEFAULT 'PENDING',
  config_error_message TEXT NOT NULL DEFAULT '',
  config_status_updated_at TEXT,
  last_seen_at TEXT,
  registered_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(organization_id, id),
  CHECK(status IN ('PENDING', 'ONLINE', 'OFFLINE', 'DISABLED'))
);
CREATE TABLE node_group_members (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  node_group_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(node_id, node_group_id),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, node_group_id) REFERENCES node_groups(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE node_listen_ips (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  listen_ip TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(organization_id, id),
  UNIQUE(organization_id, node_id, id),
  UNIQUE(node_id, listen_ip),
  CHECK(enabled IN (0, 1)),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE node_port_ranges (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  protocol TEXT NOT NULL,
  start_port INTEGER NOT NULL,
  end_port INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK(protocol IN ('TCP', 'UDP')),
  CHECK(start_port BETWEEN 1 AND 65535),
  CHECK(end_port BETWEEN 1 AND 65535),
  CHECK(start_port <= end_port),
  CHECK(enabled IN (0, 1)),
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE agent_registration_tokens (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_type TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  used_at TEXT,
  revoked_at TEXT,
  created_by_user_id TEXT,
  created_at TEXT NOT NULL,
  CHECK(agent_type IN ('NODE', 'MONITOR'))
);
CREATE TABLE agent_credentials (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_type TEXT NOT NULL,
  agent_id TEXT NOT NULL,
  credential_hash TEXT NOT NULL UNIQUE,
  registration_token_id TEXT,
  activated_at TEXT,
  revoked_at TEXT,
  rotated_at TEXT,
  created_at TEXT NOT NULL,
  CHECK(agent_type IN ('NODE', 'MONITOR'))
);
-- +goose StatementBegin
CREATE TRIGGER validate_agent_registration_tokens_agent_insert BEFORE INSERT ON agent_registration_tokens BEGIN
  SELECT RAISE(ABORT, 'NODE registration token must reference a same-organization node')
  WHERE NEW.agent_type = 'NODE'
    AND NOT EXISTS (
    SELECT 1 FROM nodes WHERE nodes.organization_id = NEW.organization_id
        AND nodes.id = NEW.agent_id
        AND nodes.deleted_at IS NULL
    );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_agent_registration_tokens_agent_update BEFORE UPDATE OF organization_id, agent_type, agent_id ON agent_registration_tokens BEGIN
  SELECT RAISE(ABORT, 'NODE registration token must reference a same-organization node')
  WHERE NEW.agent_type = 'NODE'
    AND NOT EXISTS (
    SELECT 1 FROM nodes WHERE nodes.organization_id = NEW.organization_id
        AND nodes.id = NEW.agent_id
        AND nodes.deleted_at IS NULL
    );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_agent_credentials_agent_insert BEFORE INSERT ON agent_credentials BEGIN
  SELECT RAISE(ABORT, 'NODE credential must reference a same-organization node')
  WHERE NEW.agent_type = 'NODE'
    AND NOT EXISTS (
    SELECT 1 FROM nodes WHERE nodes.organization_id = NEW.organization_id
        AND nodes.id = NEW.agent_id
        AND nodes.deleted_at IS NULL
    );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_agent_credentials_agent_update BEFORE UPDATE OF organization_id, agent_type, agent_id ON agent_credentials BEGIN
  SELECT RAISE(ABORT, 'NODE credential must reference a same-organization node')
  WHERE NEW.agent_type = 'NODE'
    AND NOT EXISTS (
    SELECT 1 FROM nodes WHERE nodes.organization_id = NEW.organization_id
        AND nodes.id = NEW.agent_id
        AND nodes.deleted_at IS NULL
    );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_agent_credentials_registration_token_insert BEFORE INSERT ON agent_credentials WHEN NEW.registration_token_id IS NOT NULL BEGIN
  SELECT RAISE(ABORT, 'agent credential registration token must reference the same agent')
  WHERE NOT EXISTS (
    SELECT 1 FROM agent_registration_tokens
    WHERE agent_registration_tokens.organization_id = NEW.organization_id
      AND agent_registration_tokens.agent_type = NEW.agent_type
      AND agent_registration_tokens.agent_id = NEW.agent_id
      AND agent_registration_tokens.id = NEW.registration_token_id
  );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_agent_credentials_registration_token_update BEFORE UPDATE OF organization_id, agent_type, agent_id, registration_token_id ON agent_credentials WHEN NEW.registration_token_id IS NOT NULL BEGIN
  SELECT RAISE(ABORT, 'agent credential registration token must reference the same agent')
  WHERE NOT EXISTS (
    SELECT 1 FROM agent_registration_tokens
    WHERE agent_registration_tokens.organization_id = NEW.organization_id
      AND agent_registration_tokens.agent_type = NEW.agent_type
      AND agent_registration_tokens.agent_id = NEW.agent_id
      AND agent_registration_tokens.id = NEW.registration_token_id
  );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER revoke_node_agent_auth_on_delete AFTER DELETE ON nodes BEGIN
  UPDATE agent_registration_tokens
  SET revoked_at = COALESCE(revoked_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
  WHERE organization_id = OLD.organization_id
    AND agent_type = 'NODE'
    AND agent_id = OLD.id;
  UPDATE agent_credentials
  SET revoked_at = COALESCE(revoked_at, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
  WHERE organization_id = OLD.organization_id
    AND agent_type = 'NODE'
    AND agent_id = OLD.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER revoke_node_agent_auth_on_soft_delete AFTER UPDATE OF deleted_at ON nodes WHEN NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL BEGIN
  UPDATE agent_registration_tokens
  SET revoked_at = COALESCE(revoked_at, NEW.deleted_at)
  WHERE organization_id = NEW.organization_id
    AND agent_type = 'NODE'
    AND agent_id = NEW.id;
  UPDATE agent_credentials
  SET revoked_at = COALESCE(revoked_at, NEW.deleted_at)
  WHERE organization_id = NEW.organization_id
    AND agent_type = 'NODE'
    AND agent_id = NEW.id;
END;
-- +goose StatementEnd
CREATE UNIQUE INDEX agent_credentials_pending_registration_token_unique ON agent_credentials(registration_token_id) WHERE registration_token_id IS NOT NULL AND activated_at IS NULL AND revoked_at IS NULL;
CREATE TABLE targets (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(organization_id, id),
  CHECK(port BETWEEN 1 AND 65535),
  CHECK(enabled IN (0, 1))
);
CREATE TABLE target_groups (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  scheduler TEXT NOT NULL DEFAULT 'PRIORITY_IPHASH',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(organization_id, id),
  CHECK(length(trim(scheduler)) > 0)
);
CREATE TABLE target_group_members (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  target_group_id TEXT NOT NULL,
  target_id TEXT NOT NULL,
  priority INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(target_group_id, target_id),
  CHECK(enabled IN (0, 1)),
  FOREIGN KEY (organization_id, target_group_id) REFERENCES target_groups(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE inbound_bindings (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  node_group_id TEXT NOT NULL,
  listen_ip TEXT NOT NULL,
  protocol TEXT NOT NULL,
  port INTEGER NOT NULL,
  match_type TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(organization_id, id),
  UNIQUE(organization_id, node_group_id, listen_ip, protocol, port, match_type),
  CHECK(protocol IN ('TCP', 'UDP', 'TCP_UDP')),
  CHECK(port BETWEEN 1 AND 65535),
  CHECK(length(trim(match_type)) > 0),
  CHECK(match_type != 'TLS_SNI' OR protocol = 'TCP'),
  CHECK(length(trim(listen_ip)) > 0),
  FOREIGN KEY (organization_id, node_group_id) REFERENCES node_groups(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE forwarding_rules (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  owner_user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'ENABLED',
  forwarding_type TEXT NOT NULL DEFAULT 'DIRECT',
  protocol TEXT NOT NULL,
  match_type TEXT NOT NULL,
  inbound_binding_id TEXT NOT NULL,
  sni_hostname TEXT,
  target_type TEXT NOT NULL DEFAULT 'TARGET',
  target_id TEXT,
  target_group_id TEXT,
  proxy_protocol_in TEXT NOT NULL DEFAULT 'NONE',
  proxy_protocol_out TEXT NOT NULL DEFAULT 'NONE',
  config_version INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT,
  UNIQUE(organization_id, id),
  CHECK(enabled IN (0, 1)),
  CHECK(status IN ('ENABLED', 'DISABLED', 'OVER_LIMIT_DISABLED')),
  CHECK(protocol IN ('TCP', 'UDP', 'TCP_UDP')),
  CHECK(length(trim(forwarding_type)) > 0),
  CHECK(length(trim(match_type)) > 0),
  CHECK(match_type != 'TLS_SNI' OR (protocol = 'TCP' AND sni_hostname IS NOT NULL AND length(trim(sni_hostname)) > 0)),
  CHECK(match_type != 'ANY_INBOUND' OR sni_hostname IS NULL),
  CHECK(protocol != 'UDP' OR (match_type = 'ANY_INBOUND' AND sni_hostname IS NULL AND proxy_protocol_in = 'NONE' AND proxy_protocol_out = 'NONE')),
  CHECK(target_type IN ('TARGET', 'TARGET_GROUP')),
  CHECK((target_type = 'TARGET' AND target_id IS NOT NULL AND target_group_id IS NULL) OR (target_type = 'TARGET_GROUP' AND target_id IS NULL AND target_group_id IS NOT NULL)),
  CHECK(proxy_protocol_in IN ('NONE', 'V1', 'V2')),
  CHECK(proxy_protocol_out IN ('NONE', 'V1', 'V2')),
  FOREIGN KEY (owner_user_id) REFERENCES "user"(id),
  FOREIGN KEY (organization_id, inbound_binding_id) REFERENCES inbound_bindings(organization_id, id),
  FOREIGN KEY (organization_id, target_id) REFERENCES targets(organization_id, id),
  FOREIGN KEY (organization_id, target_group_id) REFERENCES target_groups(organization_id, id)
);
-- +goose StatementBegin
CREATE TRIGGER validate_inbound_binding_rule_compatibility_update BEFORE UPDATE OF organization_id, protocol, match_type ON inbound_bindings BEGIN
  SELECT RAISE(ABORT, 'inbound binding update would make forwarding rules incompatible')
  WHERE EXISTS (
    SELECT 1 FROM forwarding_rules WHERE forwarding_rules.organization_id = OLD.organization_id
      AND forwarding_rules.inbound_binding_id = OLD.id
      AND forwarding_rules.deleted_at IS NULL
      AND (
        forwarding_rules.organization_id != NEW.organization_id
        OR forwarding_rules.protocol != NEW.protocol
        OR forwarding_rules.match_type != NEW.match_type
      )
  );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_forwarding_rules_inbound_insert BEFORE INSERT ON forwarding_rules BEGIN
  SELECT RAISE(ABORT, 'forwarding rule inbound binding must match protocol and match type')
  WHERE NOT EXISTS (
    SELECT 1 FROM inbound_bindings WHERE inbound_bindings.organization_id = NEW.organization_id
      AND inbound_bindings.id = NEW.inbound_binding_id
      AND inbound_bindings.protocol = NEW.protocol
      AND inbound_bindings.match_type = NEW.match_type
  );
  SELECT RAISE(ABORT, 'ANY_INBOUND rule already exists on this inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type = 'ANY_INBOUND'
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    );
  SELECT RAISE(ABORT, 'TLS_SNI rule already exists on this inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type = 'TLS_SNI'
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
        AND (
          existing_rule.match_type != 'TLS_SNI'
          OR trim(existing_binding.listen_ip, '[]') != trim(new_binding.listen_ip, '[]')
          OR (
            existing_rule.match_type = 'TLS_SNI'
            AND lower(existing_rule.sni_hostname) = lower(NEW.sni_hostname)
          )
        )
    );
  SELECT RAISE(ABORT, 'unsupported match type conflicts with existing inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type NOT IN ('ANY_INBOUND', 'TLS_SNI')
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    );
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER validate_forwarding_rules_inbound_update BEFORE UPDATE OF organization_id, enabled, status, protocol, match_type, inbound_binding_id, sni_hostname, target_type, target_id, target_group_id, deleted_at ON forwarding_rules BEGIN
  SELECT RAISE(ABORT, 'forwarding rule inbound binding must match protocol and match type')
  WHERE NOT EXISTS (
    SELECT 1 FROM inbound_bindings WHERE inbound_bindings.organization_id = NEW.organization_id
      AND inbound_bindings.id = NEW.inbound_binding_id
      AND inbound_bindings.protocol = NEW.protocol
      AND inbound_bindings.match_type = NEW.match_type
  );
  SELECT RAISE(ABORT, 'ANY_INBOUND rule already exists on this inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type = 'ANY_INBOUND'
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    );
  SELECT RAISE(ABORT, 'TLS_SNI rule already exists on this inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type = 'TLS_SNI'
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
        AND (
          existing_rule.match_type != 'TLS_SNI'
          OR trim(existing_binding.listen_ip, '[]') != trim(new_binding.listen_ip, '[]')
          OR (
            existing_rule.match_type = 'TLS_SNI'
            AND lower(existing_rule.sni_hostname) = lower(NEW.sni_hostname)
          )
        )
    );
  SELECT RAISE(ABORT, 'unsupported match type conflicts with existing inbound endpoint')
  WHERE NEW.deleted_at IS NULL
    AND NEW.enabled = 1
    AND NEW.status = 'ENABLED'
    AND NEW.match_type NOT IN ('ANY_INBOUND', 'TLS_SNI')
    AND EXISTS (
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
        AND existing_rule.enabled = 1
        AND existing_rule.status = 'ENABLED'
        AND existing_rule.deleted_at IS NULL
        AND existing_binding.node_group_id = new_binding.node_group_id
        AND (
          existing_binding.listen_ip = new_binding.listen_ip
          OR trim(existing_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
          OR trim(new_binding.listen_ip, '[]') IN ('', '0.0.0.0', '::')
        )
        AND existing_binding.port = new_binding.port
        AND (
          existing_binding.protocol = new_binding.protocol
          OR existing_binding.protocol = 'TCP_UDP'
          OR new_binding.protocol = 'TCP_UDP'
        )
    );
END;
-- +goose StatementEnd
CREATE TABLE rule_tags (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  rule_id TEXT NOT NULL,
  tag TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(rule_id, tag),
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE rule_traffic_counters (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  rule_id TEXT NOT NULL,
  node_id TEXT NOT NULL,
  bucket_start_at TEXT NOT NULL,
  bucket_granularity TEXT NOT NULL,
  upload_bytes INTEGER NOT NULL DEFAULT 0,
  download_bytes INTEGER NOT NULL DEFAULT 0,
  tcp_connections INTEGER NOT NULL DEFAULT 0,
  udp_packets INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(rule_id, node_id, bucket_start_at, bucket_granularity),
  CHECK(bucket_granularity IN ('MINUTE', 'HOUR', 'DAY')),
  CHECK(upload_bytes >= 0),
  CHECK(download_bytes >= 0),
  CHECK(tcp_connections >= 0),
  CHECK(udp_packets >= 0),
  FOREIGN KEY (organization_id, rule_id) REFERENCES forwarding_rules(organization_id, id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, node_id) REFERENCES nodes(organization_id, id) ON DELETE CASCADE
);
CREATE TABLE audit_logs (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  actor_user_id TEXT REFERENCES "user"(id),
  actor_agent_id TEXT,
  actor_roles_json TEXT NOT NULL,
  actor_permissions_json TEXT NOT NULL,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  result TEXT NOT NULL,
  error_message TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  source_ip TEXT,
  created_at TEXT NOT NULL,
  CHECK(result IN ('SUCCESS', 'FAILURE'))
);
CREATE INDEX idx_audit_logs_org_created ON audit_logs(organization_id, created_at);
CREATE INDEX idx_forwarding_rules_org_owner ON forwarding_rules(organization_id, owner_user_id);
-- +goose StatementBegin
CREATE TRIGGER prevent_audit_logs_update BEFORE UPDATE ON audit_logs BEGIN
  SELECT RAISE(ABORT, 'audit logs are append-only');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER prevent_audit_logs_delete BEFORE DELETE ON audit_logs BEGIN
  SELECT RAISE(ABORT, 'audit logs are append-only');
END;
-- +goose StatementEnd
-- +goose Down
DROP TRIGGER IF EXISTS prevent_audit_logs_delete;
DROP TRIGGER IF EXISTS prevent_audit_logs_update;
DROP TRIGGER IF EXISTS validate_node_port_range_delete_preserves_bindings;
DROP TRIGGER IF EXISTS validate_node_port_range_update_preserves_bindings;
DROP TRIGGER IF EXISTS validate_inbound_binding_rule_compatibility_update;
DROP TRIGGER IF EXISTS validate_inbound_binding_port_range_update;
DROP TRIGGER IF EXISTS validate_inbound_binding_port_range_insert;
DROP TRIGGER IF EXISTS validate_node_listen_ip_remains_enabled_for_bindings;
DROP TRIGGER IF EXISTS validate_inbound_binding_listen_ip_update;
DROP TRIGGER IF EXISTS validate_inbound_binding_listen_ip_insert;
DROP TRIGGER IF EXISTS validate_forwarding_rules_inbound_update;
DROP TRIGGER IF EXISTS validate_forwarding_rules_inbound_insert;
DROP TRIGGER IF EXISTS revoke_node_agent_auth_on_soft_delete;
DROP TRIGGER IF EXISTS revoke_node_agent_auth_on_delete;
DROP TRIGGER IF EXISTS validate_agent_credentials_registration_token_update;
DROP TRIGGER IF EXISTS validate_agent_credentials_registration_token_insert;
DROP TRIGGER IF EXISTS validate_agent_credentials_agent_update;
DROP TRIGGER IF EXISTS validate_agent_credentials_agent_insert;
DROP TRIGGER IF EXISTS validate_agent_registration_tokens_agent_update;
DROP TRIGGER IF EXISTS validate_agent_registration_tokens_agent_insert;
DROP INDEX IF EXISTS agent_credentials_pending_registration_token_unique;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS rule_traffic_counters;
DROP TABLE IF EXISTS rule_tags;
DROP TABLE IF EXISTS quotas;
DROP TABLE IF EXISTS forwarding_rules;
DROP TABLE IF EXISTS inbound_bindings;
DROP TABLE IF EXISTS target_group_members;
DROP TABLE IF EXISTS target_groups;
DROP TABLE IF EXISTS targets;
DROP TABLE IF EXISTS agent_credentials;
DROP TABLE IF EXISTS agent_registration_tokens;
DROP TABLE IF EXISTS node_port_ranges;
DROP TABLE IF EXISTS node_listen_ips;
DROP TABLE IF EXISTS node_group_members;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS node_groups;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS verification;
DROP TABLE IF EXISTS account;
DROP TABLE IF EXISTS session;
DROP TABLE IF EXISTS "user";
