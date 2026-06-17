-- Modify "redeem_codes" table: add the operator-supplied audit note and the
-- per-row classification of why a code was disabled. Both nullable + default
-- empty so existing rows stay untouched and the rollout is zero-downtime.
ALTER TABLE "redeem_codes" ADD COLUMN "note" character varying NULL DEFAULT '';
ALTER TABLE "redeem_codes" ADD COLUMN "disabled_reason" character varying NULL DEFAULT '';
