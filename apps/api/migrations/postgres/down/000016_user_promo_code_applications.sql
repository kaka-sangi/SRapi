-- Drop promo application receipts and payment order discount fields.
DROP TABLE IF EXISTS "user_promo_code_applications";
DROP INDEX IF EXISTS "paymentorder_promo_code_id";
ALTER TABLE "payment_orders" DROP COLUMN IF EXISTS "promo_code_id";
ALTER TABLE "payment_orders" DROP COLUMN IF EXISTS "discount_amount";
ALTER TABLE "payment_orders" DROP COLUMN IF EXISTS "original_amount";
