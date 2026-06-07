package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CopilotConversation is one persisted admin-copilot chat transcript, scoped to
// the admin who owns it. The transcript is stored as an opaque JSON array in
// messages_json (the frontend owns its shape). Per-admin isolation is enforced
// by always querying on admin_user_id.
type CopilotConversation struct {
	ent.Schema
}

func (CopilotConversation) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}}
}

func (CopilotConversation) Fields() []ent.Field {
	return []ent.Field{
		field.Int("admin_user_id"),
		field.String("title").Default(""),
		field.Text("messages_json").Default("[]"),
	}
}

func (CopilotConversation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("admin_user_id", "updated_at"),
	}
}
