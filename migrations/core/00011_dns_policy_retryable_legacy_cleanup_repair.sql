-- +goose Up
ALTER TABLE dns_managed_records
  ADD COLUMN IF NOT EXISTS provider_retirements_json jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose StatementBegin
DO $$
BEGIN
  IF to_regclass('dns_records') IS NOT NULL AND to_regclass('dns_managed_records') IS NOT NULL THEN
    UPDATE dns_managed_records managed
    SET provider_retirements_json = CASE
          WHEN managed.provider_retirements_json = '[]'::jsonb THEN legacy.retirements_json
          ELSE managed.provider_retirements_json || legacy.retirements_json
        END,
        updated_at = clock_timestamp()
    FROM (
      SELECT
        records.organization_id,
        records.id,
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
        ), '[]'::jsonb) AS retirements_json
      FROM dns_records records
      WHERE records.deleted_at IS NULL
    ) legacy
    WHERE managed.organization_id = legacy.organization_id
      AND managed.id = legacy.id
      AND managed.deleted_at IS NULL
      AND legacy.retirements_json <> '[]'::jsonb;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
SELECT 1;
