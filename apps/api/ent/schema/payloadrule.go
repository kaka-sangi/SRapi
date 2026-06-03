package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PayloadRule is an operator-configured request-body transform rule. Matched by
// model glob + upstream protocol, it sets/overrides/removes dotted JSON paths on
// the marshaled upstream payload just before dispatch (default / override /
// filter). params_json holds path->value for default/override, or path keys for
// filter.
type PayloadRule struct {
	ent.Schema
}

func (PayloadRule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (PayloadRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.Int("priority").Default(0),
		field.String("action"), // default | override | filter
		field.String("match_model").Default("*"),
		field.String("match_protocol").Default(""), // "" = any protocol
		field.JSON("params_json", map[string]any{}).Optional(),
	}
}

func (PayloadRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("priority"),
	}
}
