-- Remove scheduler fallback chain evidence.
ALTER TABLE "scheduler_decisions" DROP COLUMN "fallback_from_decision_id";
