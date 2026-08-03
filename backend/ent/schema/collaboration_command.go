package schema

import (
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

// CollaborationCommand stores command audit metadata without the prompt body.
type CollaborationCommand struct {
	ent.Schema
}

func (CollaborationCommand) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "collaboration_commands"},
	}
}

func (CollaborationCommand) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CollaborationCommand) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.UUID("device_id", uuid.UUID{}).
			Immutable(),
		field.String("thread_id").
			MaxLen(512).
			NotEmpty().
			Immutable(),
		field.UUID("idempotency_key", uuid.UUID{}).
			Immutable(),
		field.String("prompt_sha256").
			MaxLen(64).
			MinLen(64).
			Immutable(),
		field.Int("prompt_bytes").
			Positive().
			Immutable(),
		field.Enum("status").
			Values("accepted", "dispatched", "started", "completed", "failed", "expired").
			Default("accepted"),
		field.String("turn_id").
			MaxLen(512).
			Optional().
			Nillable(),
		field.String("error_code").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("dispatched_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("started_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CollaborationCommand) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("collaboration_commands").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		edge.From("device", CollaborationDevice.Type).
			Ref("commands").
			Field("device_id").
			Unique().
			Required().
			Immutable(),
		edge.To("charge", CollaborationCharge.Type).
			Unique(),
	}
}

func (CollaborationCommand) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "idempotency_key").Unique(),
		index.Fields("user_id", "id"),
		index.Fields("user_id", "device_id", "created_at"),
		index.Fields("user_id", "device_id", "thread_id", "status"),
		index.Fields("status", "expires_at"),
	}
}
