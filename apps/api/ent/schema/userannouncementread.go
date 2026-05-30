package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserAnnouncementRead struct {
	ent.Schema
}

func (UserAnnouncementRead) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (UserAnnouncementRead) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("announcement_id"),
		field.Time("read_at"),
	}
}

func (UserAnnouncementRead) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "announcement_id").Unique(),
		index.Fields("announcement_id"),
		index.Fields("user_id", "read_at"),
	}
}
