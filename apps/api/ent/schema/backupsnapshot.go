package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BackupSnapshot is one row of the database-backup history. The backup worker
// writes one row per attempted snapshot (status=running -> success|failed) so
// operators get real visibility into past snapshots — previously the only
// signal was AdminSettings.Backup.LastBackupAt, which collapsed all history
// into a single timestamp.
type BackupSnapshot struct {
	ent.Schema
}

func (BackupSnapshot) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (BackupSnapshot) Fields() []ent.Field {
	return []ent.Field{
		// kind is "scheduled" for worker-driven runs, "manual" for an operator-
		// triggered RunOnce. Drives the badge in the UI.
		field.String("kind").Default("scheduled"),
		// started_at is when the worker started writing the file. Indexed so
		// the list query orders by it cheaply.
		field.Time("started_at"),
		// completed_at is set when the file is fully flushed and checksummed
		// (or when the run failed); nil while status="running".
		field.Time("completed_at").Optional().Nillable(),
		// size_bytes is the on-disk size of the dump file. 0 for failed runs.
		field.Int64("size_bytes").Default(0),
		// sha256 is the hex-encoded checksum of the dump file (64 chars). Empty
		// for failed or superseded runs whose file has been wiped.
		field.String("sha256").Default("").MaxLen(64),
		// status: running | success | failed | superseded. "superseded" is set
		// by retention cleanup when it deletes the file but keeps the row.
		field.String("status").Default("running"),
		// file_path is the absolute (or worker-dir-relative) path of the dump
		// file on disk. Cleared when retention deletes the file.
		field.String("file_path").Default("").MaxLen(1024),
		// error_message captures pg_dump stderr / system errors for failed runs.
		field.String("error_message").Default("").MaxLen(1024),
		// triggered_by_user_id is the admin user that invoked a manual run.
		// 0 for scheduled runs.
		field.Int("triggered_by_user_id").Default(0),
	}
}

func (BackupSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("started_at"),
		index.Fields("status"),
	}
}
