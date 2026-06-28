package main

import "testing"

func TestCoreMigrationsPersistDocumentedRuleRoutingFields(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()
	seedTenantFixture(t, db)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_binding_documented_fields', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 10443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id,
			organization_id,
			owner_user_id,
			name,
			enabled,
			status,
			protocol,
			match_type,
			inbound_binding_id,
			sni_hostname,
			target_type,
			target_id,
			target_group_id,
			proxy_protocol_in,
			proxy_protocol_out,
			config_version,
			created_at,
			updated_at
		)
		VALUES (
			'rule_documented_fields',
			'org_a',
			'user_a',
			'Rule Documented Fields',
			1,
			'ENABLED',
			'TCP',
			'TLS_SNI',
			'inbound_binding_documented_fields',
			'app.example.com',
			'TARGET_GROUP',
			NULL,
			'target_group_a',
			'V1',
			'V2',
			0,
			'2026-01-01T00:00:00Z',
			'2026-01-01T00:00:00Z'
		)
	`)

	var inboundBindingID string
	var sniHostname string
	var proxyProtocolIn string
	var proxyProtocolOut string
	if err := db.QueryRow(`
		SELECT inbound_binding_id, sni_hostname, proxy_protocol_in, proxy_protocol_out
		FROM forwarding_rules
		WHERE id = $1
	`, fixtureUUID("rule_documented_fields")).Scan(&inboundBindingID, &sniHostname, &proxyProtocolIn, &proxyProtocolOut); err != nil {
		t.Fatalf("read documented rule fields: %v", err)
	}
	if inboundBindingID != fixtureUUID("inbound_binding_documented_fields") ||
		sniHostname != "app.example.com" ||
		proxyProtocolIn != "V1" ||
		proxyProtocolOut != "V2" {
		t.Fatalf("unexpected documented rule fields: %q %q %q %q", inboundBindingID, sniHostname, proxyProtocolIn, proxyProtocolOut)
	}

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_binding_missing_sni', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 10444, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			proxy_protocol_in, proxy_protocol_out, config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_missing_sni', 'org_a', 'user_a', 'Rule Missing SNI', 1, 'ENABLED', 'TCP', 'TLS_SNI',
			'inbound_binding_missing_sni', '', 'TARGET', 'target_a', NULL,
			'NONE', 'NONE', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_binding_bad_proxy', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 10445, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			proxy_protocol_in, proxy_protocol_out, config_version, created_at, updated_at
		)
		VALUES (
			'rule_bad_proxy_protocol', 'org_a', 'user_a', 'Rule Bad Proxy', 1, 'ENABLED', 'TCP', 'ANY_INBOUND',
			'inbound_binding_bad_proxy', NULL, 'TARGET', 'target_a', NULL,
			'V3', 'NONE', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
}

func TestCoreMigrationsEnforceInboundBindingMatchConstraints(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()
	seedTenantFixture(t, db)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_any', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 443, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_any_duplicate', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 443, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_any_primary_443', 'org_a', 'user_a', 'Rule Any Primary 443', 1, 'ENABLED', 'TCP', 'ANY_INBOUND',
			'inbound_any', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_any_duplicate_443', 'org_a', 'user_a', 'Rule Any Duplicate 443', 1, 'ENABLED', 'TCP', 'ANY_INBOUND',
			'inbound_any_duplicate', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_sni_same_endpoint', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_sni', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 8443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_udp_sni', 'org_a', 'node_group_a', '0.0.0.0', 'UDP', 9443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_feature', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 9444, 'FEATURE', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			proxy_protocol_in, proxy_protocol_out, config_version, created_at, updated_at
		)
		VALUES (
			'rule_feature_enabled', 'org_a', 'user_a', 'Rule Feature', 1, 'ENABLED', 'TCP', 'FEATURE',
			'inbound_feature', NULL, 'TARGET', 'target_a', NULL,
			'NONE', 'NONE', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_commercial_match', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 9445, 'COMMERCIAL_MATCH', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			proxy_protocol_in, proxy_protocol_out, config_version, created_at, updated_at
		)
		VALUES (
			'rule_commercial_match_disabled', 'org_a', 'user_a', 'Rule Commercial Match', 0, 'DISABLED', 'TCP', 'COMMERCIAL_MATCH',
			'inbound_commercial_match', NULL, 'TARGET', 'target_a', NULL,
			'NONE', 'NONE', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
}

func TestCoreMigrationsKeepInboundBindingsAsRuleParents(t *testing.T) {
	db := openMigratedCoreDB(t)
	defer func() { _ = db.Close() }()
	seedTenantFixture(t, db)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tls_parent', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_app', 'org_a', 'user_a', 'Rule TLS App', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_parent', 'app.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_api', 'org_a', 'user_a', 'Rule TLS API', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_parent', 'api.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_duplicate', 'org_a', 'user_a', 'Rule TLS Duplicate', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_parent', 'APP.EXAMPLE.COM', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tls_specific_overlap_parent', 'org_a', 'node_group_a', '127.0.0.1', 'TCP', 11443, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_specific_overlap_insert', 'org_a', 'user_a', 'Rule TLS Specific Overlap Insert', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_specific_overlap_parent', 'specific-overlap-insert.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_specific_overlap_enable', 'org_a', 'user_a', 'Rule TLS Specific Overlap Enable', 0, 'DISABLED', 'TCP', 'TLS_SNI',
			'inbound_tls_specific_overlap_parent', 'specific-overlap-enable.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		UPDATE forwarding_rules
		SET enabled = 1, status = 'ENABLED'
		WHERE id = 'rule_tls_specific_overlap_enable'
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_draft_duplicate', 'org_a', 'user_a', 'Rule TLS Draft Duplicate', 0, 'DISABLED', 'TCP', 'TLS_SNI',
			'inbound_tls_parent', 'APP.EXAMPLE.COM', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		UPDATE forwarding_rules
		SET enabled = 1, status = 'ENABLED'
		WHERE id = 'rule_tls_draft_duplicate'
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_missing_parent', 'org_a', 'user_a', 'Rule TLS Missing Parent', 1, 'TCP', 'TLS_SNI',
			'missing_inbound_parent', 'missing.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_mismatch', 'org_a', 'user_a', 'Rule TLS Mismatch', 1, 'TCP', 'ANY_INBOUND',
			'inbound_tls_parent', NULL, 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_commercial_blocked_by_tls', 'org_a', 'node_group_a', '127.0.0.1', 'TCP', 11443, 'COMMERCIAL_MATCH', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_commercial_blocked_by_tls', 'org_a', 'user_a', 'Rule Commercial Blocked By TLS', 1, 'TCP', 'COMMERCIAL_MATCH',
			'inbound_commercial_blocked_by_tls', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_commercial_endpoint_owner', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11444, 'COMMERCIAL_MATCH', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_commercial_endpoint_owner', 'org_a', 'user_a', 'Rule Commercial Endpoint Owner', 1, 'TCP', 'COMMERCIAL_MATCH',
			'inbound_commercial_endpoint_owner', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tls_specific_blocked_by_commercial', 'org_a', 'node_group_a', '127.0.0.1', 'TCP', 11444, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_specific_blocked_by_commercial', 'org_a', 'user_a', 'Rule TLS Specific Blocked By Commercial', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_specific_blocked_by_commercial', 'specific-commercial-blocked.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tls_blocked_by_commercial', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11444, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_blocked_by_commercial_insert', 'org_a', 'user_a', 'Rule TLS Blocked By Commercial Insert', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_blocked_by_commercial', 'commercial-blocked.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, status, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_blocked_by_commercial_enable', 'org_a', 'user_a', 'Rule TLS Blocked By Commercial Enable', 0, 'DISABLED', 'TCP', 'TLS_SNI',
			'inbound_tls_blocked_by_commercial', 'commercial-enable-blocked.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		UPDATE forwarding_rules
		SET enabled = 1, status = 'ENABLED'
		WHERE id = 'rule_tls_blocked_by_commercial_enable'
	`)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_any_parent', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11080, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_any_primary', 'org_a', 'user_a', 'Rule Any Primary', 1, 'TCP', 'ANY_INBOUND',
			'inbound_any_parent', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_any_duplicate', 'org_a', 'user_a', 'Rule Any Duplicate', 1, 'TCP', 'ANY_INBOUND',
			'inbound_any_parent', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_commercial_blocked_by_any', 'org_a', 'node_group_a', '127.0.0.1', 'TCP', 11080, 'COMMERCIAL_MATCH', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_commercial_blocked_by_any', 'org_a', 'user_a', 'Rule Commercial Blocked By Any', 1, 'TCP', 'COMMERCIAL_MATCH',
			'inbound_commercial_blocked_by_any', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tls_blocked_by_any', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11080, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_blocked_by_any', 'org_a', 'user_a', 'Rule TLS Blocked By Any', 1, 'TCP', 'TLS_SNI',
			'inbound_tls_blocked_by_any', 'blocked-by-any.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		DELETE FROM inbound_bindings
		WHERE id = 'inbound_tls_blocked_by_any'
	`)
	expectExecError(t, db, `
		UPDATE inbound_bindings
		SET match_type = 'TLS_SNI'
		WHERE id = 'inbound_any_parent'
	`)
	mustExec(t, db, `
		UPDATE forwarding_rules
		SET deleted_at = '2026-01-01T00:10:00Z'
		WHERE id = 'rule_any_primary'
	`)
	mustExec(t, db, `
		UPDATE inbound_bindings
		SET match_type = 'TLS_SNI'
		WHERE id = 'inbound_any_parent'
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, sni_hostname, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tls_reuse_after_delete', 'org_a', 'user_a', 'Rule TLS Reuse After Delete', 1, 'TCP', 'TLS_SNI',
			'inbound_any_parent', 'reused.example.com', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)

	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp', 'org_a', 'node_group_a', '0.0.0.0', 'TCP_UDP', 11081, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tcp_udp', 'org_a', 'user_a', 'Rule TCP UDP', 1, 'TCP_UDP', 'ANY_INBOUND',
			'inbound_tcp_udp', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp_tcp_conflict', 'org_a', 'node_group_a', '0.0.0.0', 'TCP', 11081, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tcp_udp_tcp_conflict', 'org_a', 'user_a', 'Rule TCP UDP TCP Conflict', 1, 'TCP', 'ANY_INBOUND',
			'inbound_tcp_udp_tcp_conflict', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp_udp_conflict', 'org_a', 'node_group_a', '0.0.0.0', 'UDP', 11081, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tcp_udp_udp_conflict', 'org_a', 'user_a', 'Rule TCP UDP UDP Conflict', 1, 'UDP', 'ANY_INBOUND',
			'inbound_tcp_udp_udp_conflict', 'TARGET', 'target_a', NULL,
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	expectExecError(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp_tls', 'org_a', 'node_group_a', '0.0.0.0', 'TCP_UDP', 11082, 'TLS_SNI', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp_proxy', 'org_a', 'node_group_a', '0.0.0.0', 'TCP_UDP', 11083, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id, proxy_protocol_in,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tcp_udp_proxy', 'org_a', 'user_a', 'Rule TCP UDP Proxy', 1, 'TCP_UDP', 'ANY_INBOUND',
			'inbound_tcp_udp_proxy', 'TARGET', 'target_a', NULL, 'V2',
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_udp_proxy', 'org_a', 'node_group_a', '0.0.0.0', 'UDP', 11085, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	expectExecError(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id, proxy_protocol_out,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_udp_proxy', 'org_a', 'user_a', 'Rule UDP Proxy', 1, 'UDP', 'ANY_INBOUND',
			'inbound_udp_proxy', 'TARGET', 'target_a', NULL, 'V1',
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
	mustExec(t, db, `
		INSERT INTO target_group_members (id, organization_id, target_group_id, target_id, priority, enabled, created_at, updated_at)
		VALUES ('target_group_member_tcp_udp', 'org_a', 'target_group_a', 'target_a', 10, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO inbound_bindings (id, organization_id, node_group_id, listen_ip, protocol, port, match_type, created_at)
		VALUES ('inbound_tcp_udp_group', 'org_a', 'node_group_a', '0.0.0.0', 'TCP_UDP', 11084, 'ANY_INBOUND', '2026-01-01T00:00:00Z')
	`)
	mustExec(t, db, `
		INSERT INTO forwarding_rules (
			id, organization_id, owner_user_id, name, enabled, protocol, match_type,
			inbound_binding_id, target_type, target_id, target_group_id,
			config_version, created_at, updated_at
		)
		VALUES (
			'rule_tcp_udp_group', 'org_a', 'user_a', 'Rule TCP UDP Group', 1, 'TCP_UDP', 'ANY_INBOUND',
			'inbound_tcp_udp_group', 'TARGET_GROUP', NULL, 'target_group_a',
			0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'
		)
	`)
}
