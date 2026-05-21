# SRapi Codex Execution Specs

## 1. Purpose

`specs/` is the execution layer for long-running Codex work. It turns the architecture and product documents in `docs/` into a concrete sequence of goals, work packages, quality gates, and progress tracking rules.

Use this directory when asking Codex to continue SRapi toward its final shape.

`docs/` answers:

- What must SRapi be?
- What are the architectural, security, data, gateway, scheduler, provider, observability, payment, and frontend rules?

`specs/` answers:

- What should Codex do next?
- Which docs must be read before changing a given area?
- What files or modules does a work package own?
- What tests and gates prove a work package is done?
- How should progress be recorded between goal turns?

## 2. Mandatory Reading Order

For any implementation goal, Codex must read in this order:

1. `specs/STATUS.md`
2. `specs/GOAL_EXECUTION_PROTOCOL.md`
3. `specs/WORK_PACKAGES.md`
4. `specs/QUALITY_GATES.md`
5. The specific docs referenced by the selected work package

If the work package touches Gateway, Scheduler, Provider Adapter, Reverse Proxy Runtime, OpenAPI, Ent schemas, payments, observability, or frontend, Codex must also read the matching `docs/*.md` files listed in that work package before editing code.

## 3. Goal Prompt Template

Use this prompt when starting or resuming long-running development:

```txt
Create a goal for SRapi: read /home/senran/Desktop/SRapi/specs/README.md, select the next pending work package from specs/STATUS.md, implement it end to end, run its quality gates, update specs/STATUS.md, and stop only when the selected work package is complete or genuinely blocked by the protocol.
```

For a specific package:

```txt
Create a goal for SRapi: implement WP-XXX from /home/senran/Desktop/SRapi/specs/WORK_PACKAGES.md according to specs/GOAL_EXECUTION_PROTOCOL.md and specs/QUALITY_GATES.md.
```

## 4. Execution Contract

Every goal turn must produce one of three outcomes:

- A completed work package with code/docs/tests updated and gates run.
- A partial but meaningful implementation that remains under the same active work package.
- A blocked status only after the same blocker repeats for three consecutive goal turns and no useful progress remains possible without user or external input.

Do not mark the global SRapi goal complete just because one work package is complete. Update `specs/STATUS.md` and move the next work package to `next_recommended`.

## 5. Source Of Truth

The final product shape is defined by:

- `specs/FINAL_STATE.md`
- `docs/PROJECT_DEVELOPMENT_PLAN.md`
- `docs/MVP_SPEC.md`
- `docs/ARCHITECTURE.md`
- `docs/AI_ENDPOINT_COMPATIBILITY.md`
- `docs/SCHEDULING_KERNEL_DESIGN.md`
- `docs/PROVIDER_ADAPTER_SPEC.md`
- `docs/REVERSE_PROXY_SPEC.md`

If a conflict appears:

1. `docs/SECURITY_MODEL.md` wins on security.
2. `docs/OPENAPI_CONTRACT.md` wins on HTTP contract and generated SDKs.
3. `docs/DATA_MODEL.md` wins on durable data.
4. `docs/MODULE_INTERFACE_CONTRACTS.md` wins on dependency direction.
5. `specs/WORK_PACKAGES.md` wins only on implementation order, not architecture.

## 6. Directory Map

| File | Role |
| --- | --- |
| `FINAL_STATE.md` | Final product and platform shape. |
| `GOAL_EXECUTION_PROTOCOL.md` | Rules for using Codex goals safely over many turns. |
| `ROADMAP.md` | Phase plan from current skeleton to final platform. |
| `WORK_PACKAGES.md` | Concrete implementation slices with docs, ownership, DoD, and gates. |
| `QUALITY_GATES.md` | Required checks by change type. |
| `REFERENCE_PROJECT_DECISIONS.md` | What to learn from `sub2api` and `CLIProxyAPI`, and what not to copy. |
| `STATUS.md` | Persistent progress ledger for future goal runs. |

