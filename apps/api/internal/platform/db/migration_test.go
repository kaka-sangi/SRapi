package db_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entschema "entgo.io/ent/dialect/sql/schema"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/ent/migrate"

	_ "github.com/mattn/go-sqlite3"
)

func TestEntSchemaAppliesToEmptyDatabase(t *testing.T) {
	const dsn = "file:srapi_ent?mode=memory&cache=shared&_fk=1"

	client := enttest.Open(t, dialect.SQLite, dsn)
	defer client.Close()

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	sort.Strings(got)

	want := []string{
		"account_availability_rollups",
		"account_group_members",
		"account_groups",
		"account_health_snapshots",
		"account_quota_snapshots",
		"affiliate_ledgers",
		"affiliate_rules",
		"api_key_groups",
		"api_keys",
		"audit_logs",
		"auth_sessions",
		"billing_ledgers",
		"capability_definitions",
		"domain_events_inboxes",
		"domain_events_outboxes",
		"email_verification_tokens",
		"entitlements",
		"error_passthrough_rules",
		"idempotency_records",
		"invite_codes",
		"invite_relationships",
		"model_alias",
		"model_provider_mappings",
		"model_rate_limits",
		"model_registries",
		"obs_alert_events",
		"obs_slo_definitions",
		"ops_system_logs",
		"password_reset_tokens",
		"payment_audit_logs",
		"payment_orders",
		"payment_provider_instances",
		"pending_oauth_sessions",
		"pricing_rules",
		"provider_accounts",
		"providers",
		"proxies",
		"quality_eval_samples",
		"quality_evaluations",
		"roles",
		"scheduler_decisions",
		"scheduler_feedbacks",
		"scheduler_request_snapshots",
		"scheduler_strategies",
		"settings",
		"subscription_plans",
		"tls_fingerprint_profiles",
		"usage_logs",
		"user_announcement_reads",
		"user_attribute_definitions",
		"user_attribute_values",
		"user_auth_identities",
		"user_promo_code_applications",
		"user_redeem_code_redemptions",
		"user_roles",
		"user_subscriptions",
		"user_totp_secrets",
		"users",
		"workspaces",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tables:\nwant: %v\ngot:  %v", want, got)
	}
}

func TestPostgresVersionedUpMigrationsMatchEntSchema(t *testing.T) {
	got := postgresUpMigrationsDDL(t)
	want, err := postgresInitialDDL(t.Context())
	if err != nil {
		t.Fatalf("generate postgres ddl from Ent schema: %v", err)
	}
	gotStatements := postgresSchemaFingerprint(got)
	wantStatements := postgresSchemaFingerprint(want)
	if !reflect.DeepEqual(gotStatements, wantStatements) {
		t.Fatalf("postgres versioned up migrations drifted from Ent schema:\nmissing from migrations: %v\nextra in migrations: %v", missingStrings(wantStatements, gotStatements), missingStrings(gotStatements, wantStatements))
	}
}

func TestPostgresInitialDownMigrationCoversInitialTables(t *testing.T) {
	up := readMigrationFile(t, "../../../migrations/postgres/up/000001_initial_schema.sql")
	down := readMigrationFile(t, "../../../migrations/postgres/down/000001_initial_schema.sql")
	want := createdTables(up)
	got := droppedTables(down)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("postgres initial down migration does not cover initial tables:\nwant: %v\ngot:  %v", want, got)
	}
}

func TestPostgresDownMigrationsCoverCreatedTables(t *testing.T) {
	up := migrationFiles(t, "../../../migrations/postgres/up")
	for _, name := range up.names {
		upSQL := readMigrationFile(t, filepath.Join("../../../migrations/postgres/up", name))
		downSQL := readMigrationFile(t, filepath.Join("../../../migrations/postgres/down", name))
		created := createdTables(upSQL)
		dropped := droppedTables(downSQL)
		if !reflect.DeepEqual(dropped, created) {
			t.Fatalf("postgres down migration %s does not cover created tables:\ncreated: %v\ndropped: %v", name, created, dropped)
		}
	}
}

func TestPostgresIncrementalMigrationsArePairedAndContiguous(t *testing.T) {
	up := migrationFiles(t, "../../../migrations/postgres/up")
	down := migrationFiles(t, "../../../migrations/postgres/down")
	if !reflect.DeepEqual(up.names, down.names) {
		t.Fatalf("postgres up/down migrations must be paired by filename:\nup:   %v\ndown: %v", up.names, down.names)
	}
	for i, number := range up.numbers {
		want := i + 1
		if number != want {
			t.Fatalf("postgres migration numbering must be contiguous from 000001: got %06d at position %d, want %06d", number, i, want)
		}
	}
}

func postgresInitialDDL(ctx context.Context) (string, error) {
	ddl, err := entschema.DDL(ctx, entschema.DDLArgs{
		Dialect: dialect.Postgres,
		Version: "16",
		Tables:  migrate.Tables,
	})
	if err != nil {
		return "", err
	}
	return normalizeSQL(ddl), nil
}

func postgresUpMigrationsDDL(t *testing.T) string {
	t.Helper()
	up := migrationFiles(t, "../../../migrations/postgres/up")
	var builder strings.Builder
	for _, name := range up.names {
		builder.WriteString(readMigrationFile(t, filepath.Join("../../../migrations/postgres/up", name)))
		builder.WriteString("\n")
	}
	return builder.String()
}

func readMigrationFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(raw)
}

type migrationFileList struct {
	names   []string
	numbers []int
}

func migrationFiles(t *testing.T, dir string) migrationFileList {
	t.Helper()
	entries, err := os.ReadDir(filepath.Clean(dir))
	if err != nil {
		t.Fatalf("read migration dir %s: %v", dir, err)
	}
	var out migrationFileList
	seen := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		number, ok := migrationNumber(entry.Name())
		if !ok {
			t.Fatalf("migration %s must be named 000001_subject.sql", entry.Name())
		}
		if existing, ok := seen[number]; ok {
			t.Fatalf("migration number %06d is used by both %s and %s", number, existing, entry.Name())
		}
		seen[number] = entry.Name()
		out.names = append(out.names, entry.Name())
		out.numbers = append(out.numbers, number)
	}
	sort.Strings(out.names)
	sort.Ints(out.numbers)
	if len(out.names) == 0 {
		t.Fatalf("no SQL migrations found in %s", dir)
	}
	return out
}

func migrationNumber(name string) (int, bool) {
	prefix, subject, ok := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
	if !ok || len(prefix) != 6 || subject == "" {
		return 0, false
	}
	number, err := strconv.Atoi(prefix)
	if err != nil || number <= 0 {
		return 0, false
	}
	for _, r := range subject {
		if r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return 0, false
	}
	return number, true
}

func normalizeSQL(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n") + "\n"
}

func sqlStatements(value string) []string {
	value = stripSQLComments(value)
	parts := strings.Split(value, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.Join(strings.Fields(part), " ")
		if statement == "" {
			continue
		}
		out = append(out, statement)
	}
	sort.Strings(out)
	return out
}

func postgresSchemaFingerprint(value string) []string {
	statements := sqlStatements(value)
	tables := map[string][]string{}
	var other []string
	droppedIndexes := map[string]struct{}{}
	for _, statement := range statements {
		if name, ok := parseDropIndexStatement(statement); ok {
			droppedIndexes[name] = struct{}{}
		}
	}
	for _, statement := range statements {
		if table, elements, ok := parseCreateTableStatement(statement); ok {
			tables[table] = append(tables[table], elements...)
			continue
		}
		if table, column, ok := parseAlterTableAddColumnStatement(statement); ok {
			tables[table] = append(tables[table], column)
			continue
		}
		if _, ok := parseDropIndexStatement(statement); ok {
			continue
		}
		if name, ok := parseCreateIndexStatement(statement); ok {
			if _, dropped := droppedIndexes[name]; dropped {
				continue
			}
			statement = canonicalCreateIndexStatement(statement)
		}
		if isDataMigrationStatement(statement) {
			continue
		}
		other = append(other, statement)
	}
	out := make([]string, 0, len(other)+len(tables))
	for table, elements := range tables {
		for i, element := range elements {
			elements[i] = strings.Join(strings.Fields(element), " ")
		}
		sort.Strings(elements)
		out = append(out, `CREATE TABLE "`+table+`" (`+strings.Join(elements, ", ")+`)`)
	}
	out = append(out, other...)
	sort.Strings(out)
	return out
}

func parseCreateTableStatement(statement string) (string, []string, bool) {
	match := regexp.MustCompile(`(?i)^CREATE TABLE (?:IF NOT EXISTS )?"([^"]+)" \((.*)\)$`).FindStringSubmatch(statement)
	if len(match) != 3 {
		return "", nil, false
	}
	return match[1], splitSQLList(match[2]), true
}

func parseAlterTableAddColumnStatement(statement string) (string, string, bool) {
	match := regexp.MustCompile(`(?i)^ALTER TABLE "([^"]+)" ADD COLUMN (?:IF NOT EXISTS )?(.*)$`).FindStringSubmatch(statement)
	if len(match) != 3 {
		return "", "", false
	}
	return match[1], match[2], true
}

func parseCreateIndexStatement(statement string) (string, bool) {
	match := regexp.MustCompile(`(?i)^CREATE (?:UNIQUE )?INDEX (?:IF NOT EXISTS )?"([^"]+)" ON `).FindStringSubmatch(statement)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func canonicalCreateIndexStatement(statement string) string {
	return regexp.MustCompile(`(?i)^(CREATE (?:UNIQUE )?INDEX) IF NOT EXISTS `).ReplaceAllString(statement, `$1 `)
}

func parseDropIndexStatement(statement string) (string, bool) {
	match := regexp.MustCompile(`(?i)^DROP INDEX "([^"]+)"$`).FindStringSubmatch(statement)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func isDataMigrationStatement(statement string) bool {
	upper := strings.ToUpper(statement)
	return strings.HasPrefix(upper, "INSERT INTO ") || strings.HasPrefix(upper, "UPDATE ")
}

func splitSQLList(value string) []string {
	var out []string
	start := 0
	depth := 0
	inDoubleQuote := false
	inSingleQuote := false
	for i, r := range value {
		switch {
		case inSingleQuote:
			if r == '\'' {
				inSingleQuote = false
			}
		case inDoubleQuote:
			if r == '"' {
				inDoubleQuote = false
			}
		default:
			switch r {
			case '\'':
				inSingleQuote = true
			case '"':
				inDoubleQuote = true
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case ',':
				if depth == 0 {
					out = append(out, strings.TrimSpace(value[start:i]))
					start = i + 1
				}
			}
		}
	}
	if tail := strings.TrimSpace(value[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func stripSQLComments(value string) string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func createdTables(sql string) []string {
	return quotedIdentifierMatches(sql, regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?"([^"]+)"`))
}

func droppedTables(sql string) []string {
	return quotedIdentifierMatches(sql, regexp.MustCompile(`(?i)DROP\s+TABLE\s+IF\s+EXISTS\s+"([^"]+)"`))
}

func quotedIdentifierMatches(sql string, pattern *regexp.Regexp) []string {
	matches := pattern.FindAllStringSubmatch(sql, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			out = append(out, match[1])
		}
	}
	sort.Strings(out)
	return out
}

func missingStrings(want []string, got []string) []string {
	gotSet := make(map[string]struct{}, len(got))
	for _, value := range got {
		gotSet[value] = struct{}{}
	}
	var missing []string
	for _, value := range want {
		if _, ok := gotSet[value]; !ok {
			missing = append(missing, value)
		}
	}
	return missing
}
