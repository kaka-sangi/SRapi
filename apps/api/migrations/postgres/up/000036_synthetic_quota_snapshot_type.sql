-- Rename locally-derived quota snapshots so runtime scheduling can ignore
-- synthetic quota evidence by quota_type without legacy heuristics.
UPDATE "account_quota_snapshots"
SET "quota_type" = 'synthetic_monthly_tokens'
WHERE "quota_type" = 'monthly_tokens'
  AND "remaining" = 'unlimited'
  AND "quota_limit" = 'unlimited';
