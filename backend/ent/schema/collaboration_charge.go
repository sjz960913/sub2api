package schema

import (
	"time"

	"github.com/google/uuid"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CollaborationCharge is an immutable direct-charge audit record.
// The collaboration product has no hold, settlement, or refund states.
type CollaborationCharge struct {
	ent.Schema
}

func (CollaborationCharge) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "collaboration_charges"},
	}
}

func (CollaborationCharge) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("command_id", uuid.UUID{}).
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.String("amount").
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.String("currency").
			MaxLen(3).
			Default("USD").
			Immutable(),
		field.Enum("status").
			Values("charged").
			Default("charged").
			Immutable(),
		field.String("balance_before").
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.String("balance_after").
			SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}).
			Immutable(),
		field.String("reason").
			MaxLen(128).
			Optional().
			Nillable().
			Immutable(),
		field.Time("charged_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CollaborationCharge) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("command", CollaborationCommand.Type).
			Ref("charge").
			Field("command_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("collaboration_charges").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (CollaborationCharge) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("command_id").Unique(),
		index.Fields("user_id", "charged_at"),
	}
}
