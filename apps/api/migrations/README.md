# Migrations

This directory is reserved for versioned Atlas/Ent migrations.

Current harness verification is exposed through `make migration-check`. It applies the Ent schema to an empty database, checks the expected MVP tables are created, verifies the initial PostgreSQL up migration has not drifted from the Ent schema, and verifies the initial down migration covers the current Ent table list.
