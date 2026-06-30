-- +goose Up
ALTER TABLE target_group_members
  ADD COLUMN IF NOT EXISTS weight integer NOT NULL DEFAULT 1;

ALTER TABLE target_group_members
  ADD CONSTRAINT target_group_members_weight_non_negative CHECK (weight >= 0) NOT VALID;

ALTER TABLE target_group_members
  VALIDATE CONSTRAINT target_group_members_weight_non_negative;

-- +goose Down
ALTER TABLE target_group_members
  DROP CONSTRAINT IF EXISTS target_group_members_weight_non_negative;

ALTER TABLE target_group_members
  DROP COLUMN IF EXISTS weight;
