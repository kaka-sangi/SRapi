ALTER TABLE "payment_orders" DROP COLUMN IF EXISTS "payable_amount";
ALTER TABLE "payment_orders" DROP COLUMN IF EXISTS "fee_amount";

ALTER TABLE "payment_provider_instances" DROP COLUMN IF EXISTS "weight";
ALTER TABLE "payment_provider_instances" DROP COLUMN IF EXISTS "fee_rate";
