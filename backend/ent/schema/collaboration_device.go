package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/google/uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CollaborationDevice stores one user-owned PC companion installation.
type CollaborationDevice struct {
	ent.Schema
}

func (CollaborationDevice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "collaboration_devices"},
	}
}

func (CollaborationDevice) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CollaborationDevice) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.String("installation_id_hash").
			MaxLen(80).
			NotEmpty(),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.Enum("platform").
			Values("windows", "macos", "linux"),
		field.String("platform_version").
			MaxLen(128).
			Optional().
			Nillable(),
		field.String("companion_version").
			MaxLen(64).
			NotEmpty(),
		field.String("codex_version").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Int("protocol_version"),
		field.Enum("status").
			Values("offline", "online", "degraded", "revoked").
			Default("offline"),
		field.JSON("capabilities", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("last_seen_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("revoked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("registered_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CollaborationDevice) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("collaboration_devices").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		edge.To("sync_requests", CollaborationSyncRequest.Type),
		edge.To("commands", CollaborationCommand.Type),
	}
}

func (CollaborationDevice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "installation_id_hash").Unique(),
		index.Fields("user_id", "status"),
		index.Fields("user_id", "updated_at"),
		index.Fields("last_seen_at"),
		index.Fields("revoked_at"),
	}
}
