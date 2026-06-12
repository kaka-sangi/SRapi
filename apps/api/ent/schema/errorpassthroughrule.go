package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ErrorPassthroughRule is an operator-managed, global rule that decides whether
// a provider's raw upstream error message is exposed to the caller or masked
// behind a generic gateway message. It centralizes what was previously only
// configurable via per-account / per-provider metadata.
type ErrorPassthroughRule struct {
	ent.Schema
}

func (ErrorPassthroughRule) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ErrorPassthroughRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Bool("enabled").Default(true),
		field.Int("priority").Default(0),         // lower priority evaluated first
		field.String("action").Default("expose"), // expose | mask
		field.JSON("match_status_codes", []int{}).Optional(),
		field.JSON("match_classes", []string{}).Optional(),
		field.JSON("match_keywords", []string{}).Optional(),
		field.Int("response_status").Optional().Nillable(),
		field.String("custom_message").Default(""),
	}
}

func (ErrorPassthroughRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled", "priority"),
	}
}
