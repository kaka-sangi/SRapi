# SRapi Kubernetes Skeleton

This directory is a production starting point, not a complete Helm chart.

The API deployment intentionally starts at one replica. Unlock multiple API
replicas only after a scale-up runbook verifies process-local `/metrics`
monotonicity, worker leader-gate skips, and database/Redis pool sizing under the
target replica count. The current codebase has the required worker gate:

- `apps/api/internal/platform/leadergate` uses PostgreSQL `pg_try_advisory_lock`.
- `apps/api/internal/app` creates a worker guard from the shared SQL handle.
- periodic workers run through `workers/runonceguard`, so non-leader replicas
  skip side-effecting or expensive worker passes.

Before applying these manifests in production:

- Replace image references with immutable release tags.
- Provide `srapi-database`, `srapi-redis`, and `srapi-secrets` from a secret
  manager or sealed-secret flow.
- Use managed PostgreSQL with automatic backups and PITR, or an operator with an
  equivalent RPO/RTO plan.
- Use managed Redis, Redis Sentinel, or an equivalent failover topology.
- Verify `replicas * DATABASE_MAX_OPEN_CONNS` and `replicas * REDIS_POOL_SIZE`
  fit the database and Redis service limits.
- Run the scale-up check with `kubectl scale deployment/srapi-api --replicas=N`
  before enabling `api-hpa.yaml`.
- Keep readiness probes on `/readyz`; rolling updates must not route gateway
  traffic to pods with failed PostgreSQL `SELECT 1` or Redis `PING`.

Operational signals to watch during scale-up:

- ready API pod count and `up{job="srapi-api"}`.
- PostgreSQL active/max connection ratio.
- Redis connected clients, `PING` latency, and rejected connections.
- gateway 5xx rate and p99 latency.
- worker logs for leader-gate acquisition skips versus actual runs.
