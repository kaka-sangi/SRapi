package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// MonitorRequestTemplate is a reusable synthetic-probe request body, headers,
// and expectation set that can be applied to one or more monitor definitions in
// bulk. It mirrors the request-override shape of MonitorDefinition so a template
// can seed many monitors with one consistent probe.
type MonitorRequestTemplate struct {
	ent.Schema
}

func (MonitorRequestTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (MonitorRequestTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("description").Default(""),
		field.String("request_method").Default(""),
		field.String("request_url").Default(""),
		field.JSON("request_headers", map[string]string{}).Optional(),
		field.String("request_body").Default(""),
		field.JSON("expected_status_codes", []int{}).Optional(),
		field.String("response_json_path").Default(""),
		field.String("response_contains").Default(""),
	}
}

func (MonitorRequestTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
	}
}
