# SRapi Goal Execution Protocol

## 1. Goal Model

SRapi development is a sequence of bounded work packages. A Codex goal should target exactly one work package at a time unless the user explicitly asks for a broader milestone.

The active work package is selected from `specs/STATUS.md`.

Codex must not skip ahead just because another task looks easier. If the selected package is blocked, Codex should first reduce the package to a useful smaller deliverable inside the same package.

## 2. Start Of Every Goal Turn

Codex must:

1. Check the active goal status if goal tooling is available.
2. Read `specs/STATUS.md`.
3. Read the selected package entry in `specs/WORK_PACKAGES.md`.
4. Read `specs/QUALITY_GATES.md`.
5. Read all `docs/*.md` files referenced by that package.
6. Inspect current code before editing.
7. Run `git status --short` and preserve unrelated user changes.

## 3. Worktree Rules

Codex must:

- Never revert unrelated user changes.
- Never reset the worktree.
- Never edit generated files directly when a generator is the source of truth.
- Prefer small, reviewable changes.
- Keep docs and code aligned.
- Update `specs/STATUS.md` only for real progress.

If unrelated files are dirty, ignore them unless they block the selected package.

## 4. Implementation Loop

For each work package:

1. Confirm the package objective and owned modules.
2. Identify source-of-truth docs.
3. Inspect nearby code, tests, generated artifacts, migrations, and OpenAPI contract.
4. Make the smallest coherent implementation.
5. Add or update focused tests.
6. Run the package-specific gates.
7. Fix failures caused by the change.
8. Update `specs/STATUS.md` with:
   - completed package ID if done
   - current package if still in progress
   - next recommended package
   - gates run
   - known residual risks

## 5. Completion Rule

A work package is complete only when:

- Its Definition of Done in `WORK_PACKAGES.md` is satisfied.
- The relevant quality gates in `QUALITY_GATES.md` pass, or any skipped gate has a concrete reason recorded in `STATUS.md`.
- Generated artifacts are in sync.
- Docs changed by behavior changes are updated.
- No required follow-up inside the package remains.

The overall SRapi goal is complete only when all final-state phases in `ROADMAP.md` are complete.

## 6. Blocked Rule

Codex may mark a goal blocked only when the same blocker has repeated for at least three consecutive goal turns and there is no smaller useful step left inside the selected package.

Examples of real blockers:

- Required upstream credentials are unavailable and no mock path can validate the behavior.
- A missing product decision changes persistent schema or public API semantics.
- Local infrastructure cannot start and no unit-level progress remains possible.

Not blockers:

- Tests are failing.
- The implementation is large.
- More documentation would be helpful.
- A refactor is tedious.

## 7. Safe Autonomy

Codex should make conservative choices using existing SRapi rules:

- OpenAPI-first for HTTP surface.
- Ent schema plus migrations for durable data.
- Module contracts for cross-module calls.
- Provider-neutral Scheduler core.
- Reverse Proxy Runtime for non-API-key account classes.
- PostgreSQL as source of truth; Redis as rebuildable runtime state.
- Security and observability built into the first implementation, not deferred when touching sensitive paths.

Ask the user only when a choice changes business policy, external compliance posture, or irreversible schema/API semantics.

## 8. Standard Status Update Format

When updating `specs/STATUS.md`, use:

```txt
last_completed:
  - WP-XXX: short result
current:
  package: WP-YYY
  status: pending | in_progress | blocked
next_recommended: WP-ZZZ
last_gates:
  - command or check name: pass | failed | skipped, reason
notes:
  - short residual risk or decision
```

## 9. Goal Prompt Shortcuts

Resume the next package:

```txt
继续 SRapi goal：按 specs/STATUS.md 的 next_recommended 执行，遵守 specs/GOAL_EXECUTION_PROTOCOL.md。
```

Run one package:

```txt
执行 SRapi WP-XXX：读 specs/WORK_PACKAGES.md、相关 docs、实现、测试、更新 STATUS。
```

Audit current state:

```txt
审计 SRapi 当前实现相对 specs/FINAL_STATE.md 和 specs/ROADMAP.md 的差距，更新 specs/STATUS.md，不做代码改动。
```

