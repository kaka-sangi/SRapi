-- Roll back the synthetic quota snapshot type rename.
UPDATE "account_quota_snapshots"
SET "quota_type" = 'monthly_tokens'
WHERE "quota_type" = 'synthetic_monthly_tokens'
  AND "remaining" = 'unlimited'
  AND "quota_limit" = 'unlimited';
