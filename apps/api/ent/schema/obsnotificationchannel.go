package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ObsNotificationChannel stores SRapi-native Ops alert notification channels.
type ObsNotificationChannel struct {
	ent.Schema
}

func (ObsNotificationChannel) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (ObsNotificationChannel) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.String("channel_type").Default("email"),
		field.String("status").Default("active"),
		field.String("min_severity").Default("warning"),
		field.JSON("config_json", map[string]any{}).Optional(),
		field.Bool("send_resolved").Default(true),
	}
}

func (ObsNotificationChannel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "channel_type"),
		index.Fields("name").Unique(),
	}
}
