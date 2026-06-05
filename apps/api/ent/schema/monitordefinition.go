package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MonitorDefinition is an operator-managed synthetic probe definition. It
// elevates the previously config-map-driven probe service into first-class,
// admin-managed monitors that target an account / group / provider / model and
// carry a custom upstream request (URL / headers / body / expected-status /
// json-path). Scope identifies what to probe; scope_ref is the matching id or
// glob for that scope.
type MonitorDefinition struct {
	ent.Schema
}

func (MonitorDefinition) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (MonitorDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.String("scope").Default("account"), // account | group | provider | model
		field.String("scope_ref").Default(""),    // account/group/provider id, or model glob
		field.Int("interval_seconds").Default(300),
		field.String("model").Default(""),          // optional explicit probe model
		field.String("request_method").Default(""), // "" inherits config default
		field.String("request_url").Default(""),    // custom probe URL override
		field.JSON("request_headers", map[string]string{}).Optional(),
		field.String("request_body").Default(""), // JSON body override
		field.JSON("expected_status_codes", []int{}).Optional(),
		field.String("response_json_path").Default(""), // JSON-path expectation
		field.String("response_contains").Default(""),  // substring expectation
	}
}

func (MonitorDefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("scope"),
	}
}
