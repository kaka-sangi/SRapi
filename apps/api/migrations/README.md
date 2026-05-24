# Migrations

SRapi uses Ent as the schema source of truth and Atlas for versioned PostgreSQL
migration planning.

Current release migrations live in split directories:

```txt
postgres/up/000001_initial_schema.sql
postgres/down/000001_initial_schema.sql
```

New migrations must keep the same basename in both directories. For example,
`postgres/up/000002_auth_sessions.sql` must be paired with
`postgres/down/000002_auth_sessions.sql`.

Generate the next up migration after changing `apps/api/ent/schema`:

```sh
make ent-generate
make migration-diff MIGRATION_NAME=000002_auth_sessions
```

`make migration-diff` requires `MIGRATION_NAME` to use SRapi's six-digit
sequence format and refuses `000001`, which is reserved for the initial schema.
It runs Atlas against `apps/api/atlas.hcl`, replaying the existing `postgres/up`
directory into a PostgreSQL 16 dev database before comparing it with
`ent://ent/schema`. Atlas may plan with its own timestamp version internally;
the Makefile normalizes the generated file back to SRapi's
`00000N_subject.sql` filename. Review the generated SQL, add the matching down
migration manually, then run:

```sh
make migration-hash
make migration-check
```

`make migration-check` applies the Ent schema to an empty database, checks the
expected MVP tables, verifies `000001_initial_schema.sql` has not drifted from
the current Ent schema while SRapi is still pre-release, verifies the initial
down migration covers the current Ent table list, and checks that every
PostgreSQL up migration has a matching down migration with contiguous numbering.
