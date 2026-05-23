# AGENTS.md

## Prime Directive
Write maintainable, simple, boring code. Prefer clarity over cleverness.
Do not introduce unnecessary abstraction, global state, hidden side effects, or large rewrites.

## Repository Rules
- Before editing, inspect the existing architecture and follow existing patterns.
- Do not create a new framework/layer/helper unless at least 2 existing call sites need it.
- Prefer small, local changes over broad rewrites.
- Keep each change focused on one goal.
- Do not mix feature work, refactor, formatting, and dependency upgrades in the same change.

## Code Quality Rules
- Functions should be small and single-purpose.
- Avoid deep nesting; prefer early returns.
- Use explicit names; avoid vague names like data, result, manager, helper unless domain-specific.
- No duplicated business logic. Extract only when duplication is real, not speculative.
- Public APIs must be documented.
- Complex private logic needs a short explanation comment.
- Error handling must be explicit and actionable.
- Do not swallow exceptions silently.
- Do not add dead code, unused parameters, unused files, or speculative TODOs.

## Architecture Constraints
- Preserve existing public API unless the task explicitly requires changing it.
- Preserve backward compatibility unless told otherwise.
- Do not introduce new dependencies without explaining why existing tools are insufficient.
- Do not change database/schema/protocol behavior without migration notes and tests.
- Do not make security-sensitive changes without calling them out.

## Testing and Verification
Before saying the task is complete:
- Add or update tests for changed behavior.
- Run the smallest relevant test suite.
- Run lint/format/typecheck if available.
- Review the diff for unnecessary changes.
- Summarize what changed, what was tested, and any remaining risk.

## Done Means
A task is done only when:
- The requested behavior works.
- Tests or checks pass, or failures are explained.
- The diff is minimal and easy to review.
- No unrelated files were changed.
- No new obvious code smells were introduced.

## When Unsure
Ask before making architectural changes.
If a change becomes larger than expected, stop and propose a smaller plan first.
