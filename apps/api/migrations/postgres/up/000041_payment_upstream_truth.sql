ALTER TABLE "payment_provider_instances" ADD COLUMN "fee_rate" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "payment_provider_instances" ADD COLUMN "weight" bigint NOT NULL DEFAULT 1;

ALTER TABLE "payment_orders" ADD COLUMN "fee_amount" character varying NOT NULL DEFAULT '0.00000000';
ALTER TABLE "payment_orders" ADD COLUMN "payable_amount" character varying NOT NULL DEFAULT '0.00000000';

UPDATE "payment_orders"
SET "payable_amount" = "amount"
WHERE "payable_amount" = '0.00000000';
