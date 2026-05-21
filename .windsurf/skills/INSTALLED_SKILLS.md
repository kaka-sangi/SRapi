# SRapi High-Star / Official Windsurf Skills

The previous low-signal skills were removed and replaced with selected official/high-star compatible Agent Skills.

## Selection Policy

- Prefer official source or repositories with 10k+ GitHub stars.
- Prefer skills directly useful for SRapi: OpenAPI/API design, frontend design, webapp testing, Docker/dev infra, database/schema work, security, observability/SLO, LLM cost optimization, and release gates.
- Do not execute remote install scripts.
- Keep the installed set small enough to avoid noisy context.

## Installed Sources

| Source | Role | Stars checked at audit time |
| --- | --- | ---: |
| https://github.com/anthropics/skills | Official Agent Skills | 138135 |
| https://github.com/alirezarezvani/claude-skills | High-star multi-tool skills; includes Windsurf support | 15636 |

## Installed Skills

### Official Anthropic Skills

- frontend-design
- webapp-testing
- mcp-builder
- web-artifacts-builder
- claude-api
- theme-factory

### High-Star Engineering Skills

- api-design-reviewer
- api-test-suite-builder
- codebase-onboarding
- database-schema-designer
- spec-driven-workflow
- sql-database-assistant
- secrets-vault-manager
- ship-gate
- self-eval
- skill-security-auditor
- ci-cd-pipeline-builder
- runbook-generator
- tech-debt-tracker
- docker-development
- security-guidance
- llm-cost-optimizer
- slo-architect

## Per-Skill Star Audit

GitHub stars are counted per repository, not per skill subdirectory. The table below shows the source repository star count for each installed skill.

| Skill | Source repository | Source repository stars | Independent skill stars |
| --- | --- | ---: | --- |
| frontend-design | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| webapp-testing | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| mcp-builder | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| web-artifacts-builder | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| claude-api | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| theme-factory | https://github.com/anthropics/skills | 138135 | N/A, repository subdirectory |
| api-design-reviewer | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| api-test-suite-builder | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| codebase-onboarding | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| database-schema-designer | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| spec-driven-workflow | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| sql-database-assistant | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| secrets-vault-manager | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| ship-gate | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| self-eval | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| skill-security-auditor | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| ci-cd-pipeline-builder | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| runbook-generator | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| tech-debt-tracker | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| docker-development | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| security-guidance | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| llm-cost-optimizer | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |
| slo-architect | https://github.com/alirezarezvani/claude-skills | 15636 | N/A, repository subdirectory |

## Explicitly Removed Low-Star Sources

- https://github.com/falconnt/windsurf-skills — 1 star at audit time
- https://github.com/rogelioGuerrero/windsurf-skills — 0 stars at audit time
