-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prism_legacy_uuid(input text)
RETURNS uuid AS $$
  SELECT (
    substr(md5(input), 1, 8) || '-' ||
    substr(md5(input), 9, 4) || '-' ||
    substr(md5(input), 13, 4) || '-' ||
    substr(md5(input), 17, 4) || '-' ||
    substr(md5(input), 21, 12)
  )::uuid;
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

ALTER TABLE dns_managed_records
  ADD COLUMN IF NOT EXISTS provider_retirements_json jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('dns_records') IS NOT NULL THEN
    INSERT INTO dns_credential_zones (
      id, organization_id, dns_credential_id, zone_id, zone_name, status,
      last_synced_at, created_at, updated_at
    )
    SELECT
      prism_legacy_uuid('legacy-zone:' || records.organization_id::text || ':' || records.dns_credential_id::text || ':' || records.zone),
      records.organization_id,
      records.dns_credential_id,
      records.zone,
      lower(records.zone),
      'UNKNOWN',
      clock_timestamp(),
      clock_timestamp(),
      clock_timestamp()
    FROM (
      SELECT DISTINCT organization_id, dns_credential_id, zone
      FROM dns_records
      WHERE deleted_at IS NULL
        AND trim(zone) <> ''
    ) records
    WHERE NOT EXISTS (
      SELECT 1
      FROM dns_credential_zones zones
      WHERE zones.organization_id = records.organization_id
        AND zones.dns_credential_id = records.dns_credential_id
        AND zones.zone_id = records.zone
    )
    ON CONFLICT DO NOTHING;

    INSERT INTO dns_managed_records (
      id, organization_id, dns_credential_id, credential_zone_id, zone_id, zone_name,
      record_host, record_name, record_type, ttl, proxied,
      last_applied_values_json, provider_retirements_json, last_evaluation_status, last_evaluation_error, last_diagnostics_json,
      last_evaluated_at, last_applied_at, created_at, updated_at
    )
    SELECT
      records.id,
      records.organization_id,
      records.dns_credential_id,
      zones.id,
      zones.zone_id,
      zones.zone_name,
      CASE
        WHEN lower(records.record_name) = lower(zones.zone_name) THEN '@'
        WHEN lower(records.record_name) LIKE '%.' || lower(zones.zone_name) THEN left(records.record_name, greatest(length(records.record_name) - length(zones.zone_name) - 1, 0))
        ELSE records.record_name
      END,
      records.record_name,
      records.record_type,
      60,
      false,
      CASE
        WHEN records.provider_delete_pending_at IS NOT NULL THEN '[]'::jsonb
        ELSE COALESCE(records.last_applied_values_json, '[]'::jsonb)
      END,
      COALESCE((
        SELECT jsonb_agg(retirement_payload ORDER BY sort_order)
        FROM (
          SELECT
            1 AS sort_order,
            jsonb_build_object(
              'provider', retire_credentials.provider,
              'encrypted_secret', retire_credentials.encrypted_secret,
              'zone', records.pending_retire_zone,
              'record_name', records.pending_retire_record_name,
              'record_type', records.pending_retire_record_type,
              'ttl', 60,
              'proxied', false,
              'created_at', COALESCE(records.pending_retire_at, clock_timestamp())::text
            ) AS retirement_payload
          FROM dns_credentials retire_credentials
          WHERE retire_credentials.organization_id = records.organization_id
            AND retire_credentials.id = records.pending_retire_dns_credential_id
            AND records.pending_retire_at IS NOT NULL
            AND records.pending_retire_dns_credential_id IS NOT NULL
            AND trim(COALESCE(records.pending_retire_zone, '')) <> ''
            AND trim(COALESCE(records.pending_retire_record_name, '')) <> ''
            AND records.pending_retire_record_type IN ('A', 'AAAA', 'CNAME')
            AND jsonb_typeof(COALESCE(records.pending_retire_values_json, '[]'::jsonb)) = 'array'
            AND jsonb_array_length(COALESCE(records.pending_retire_values_json, '[]'::jsonb)) > 0
          UNION ALL
          SELECT
            2 AS sort_order,
            jsonb_build_object(
              'provider', delete_credentials.provider,
              'encrypted_secret', delete_credentials.encrypted_secret,
              'zone', records.zone,
              'record_name', records.record_name,
              'record_type', records.record_type,
              'ttl', 60,
              'proxied', false,
              'created_at', COALESCE(records.provider_delete_pending_at, clock_timestamp())::text
            ) AS retirement_payload
          FROM dns_credentials delete_credentials
          WHERE delete_credentials.organization_id = records.organization_id
            AND delete_credentials.id = records.dns_credential_id
            AND records.provider_delete_pending_at IS NOT NULL
            AND jsonb_typeof(COALESCE(records.last_applied_values_json, '[]'::jsonb)) = 'array'
            AND jsonb_array_length(COALESCE(records.last_applied_values_json, '[]'::jsonb)) > 0
        ) retirements
      ), '[]'::jsonb),
      'PENDING',
      '',
      '[]'::jsonb,
      NULL,
      records.last_applied_at,
      records.created_at,
      records.updated_at
    FROM dns_records records
    JOIN dns_credential_zones zones
      ON zones.organization_id = records.organization_id
     AND zones.dns_credential_id = records.dns_credential_id
     AND zones.zone_id = records.zone
    WHERE records.deleted_at IS NULL
      AND records.id::text = (
        SELECT min(same_type.id::text)
        FROM dns_records same_type
        WHERE same_type.organization_id = records.organization_id
          AND same_type.zone = records.zone
          AND lower(same_type.record_name) = lower(records.record_name)
          AND same_type.record_type = records.record_type
          AND same_type.deleted_at IS NULL
      )
      AND (
        records.record_type = 'CNAME'
        OR NOT EXISTS (
          SELECT 1
          FROM dns_records cname
          WHERE cname.organization_id = records.organization_id
            AND cname.zone = records.zone
            AND lower(cname.record_name) = lower(records.record_name)
            AND cname.deleted_at IS NULL
            AND cname.record_type = 'CNAME'
        )
      )
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO dns_instances (
      id, organization_id, managed_record_id, name, priority, enabled, node_group_ids_json,
      answer_count, condition_json, action_json, notification_channel_ids_json,
      last_output_values_json, last_status, last_diagnostics_json, created_at, updated_at
    )
    SELECT
      prism_legacy_uuid('legacy-dns-instance:' || records.id::text),
      records.organization_id,
      records.id,
      'Migrated static DNS values',
      100,
      true,
      '[]'::jsonb,
      -1,
      '{}'::jsonb,
      CASE
        WHEN records.record_type = 'CNAME' THEN jsonb_build_object('type', 'SET_STATIC_CNAME', 'value', COALESCE(records.desired_values_json->>0, ''))
        ELSE jsonb_build_object('type', 'SET_STATIC_ADDRESSES', 'values', COALESCE(records.desired_values_json, '[]'::jsonb))
      END,
      '[]'::jsonb,
      COALESCE(records.desired_values_json, '[]'::jsonb),
      'PENDING',
      '[]'::jsonb,
      records.created_at,
      records.updated_at
    FROM dns_records records
    WHERE records.deleted_at IS NULL
      AND records.provider_delete_pending_at IS NULL
      AND jsonb_typeof(COALESCE(records.desired_values_json, '[]'::jsonb)) = 'array'
      AND jsonb_array_length(COALESCE(records.desired_values_json, '[]'::jsonb)) > 0
      AND EXISTS (
        SELECT 1
        FROM dns_managed_records managed
        WHERE managed.organization_id = records.organization_id
          AND managed.id = records.id
          AND managed.deleted_at IS NULL
      )
    ON CONFLICT (id) DO NOTHING;
  END IF;
END $$;
-- +goose StatementEnd

DELETE FROM health_events
WHERE event_type IN ('DNS_FAILOVER', 'DNS_DELETE_OFFLINE', 'DNS_DELETE_ALL', 'DNS_RESTORE');

UPDATE health_evaluation_rules
SET deleted_at = COALESCE(deleted_at, clock_timestamp()),
    updated_at = clock_timestamp()
WHERE deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM health_events
    WHERE health_events.organization_id = health_evaluation_rules.organization_id
      AND health_events.health_evaluation_rule_id = health_evaluation_rules.id
      AND health_events.deleted_at IS NULL
  );

ALTER TABLE health_events
  DROP CONSTRAINT IF EXISTS health_events_event_type_check;

ALTER TABLE health_events
  ADD CONSTRAINT health_events_event_type_check
  CHECK (event_type IN ('WEBHOOK', 'EMAIL'));

DROP TABLE IF EXISTS dns_records;

DROP FUNCTION IF EXISTS prism_legacy_uuid(text);

-- +goose Down
SELECT 1;
