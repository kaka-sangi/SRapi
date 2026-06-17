ALTER TABLE "redeem_codes"
  DROP COLUMN IF EXISTS "note",
  DROP COLUMN IF EXISTS "disabled_reason";
