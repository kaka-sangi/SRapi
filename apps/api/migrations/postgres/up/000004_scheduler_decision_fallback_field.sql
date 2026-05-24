-- Add scheduler fallback chain evidence.
ALTER TABLE "scheduler_decisions" ADD COLUMN "fallback_from_decision_id" bigint NULL;
