-- Add a low-sensitive scheduler explanation sentence for admin diagnostics.
ALTER TABLE "scheduler_decisions" ADD COLUMN "selection_rationale" character varying NOT NULL DEFAULT '';
