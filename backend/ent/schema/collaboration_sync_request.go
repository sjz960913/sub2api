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

// CollaborationSyncRequest stores authoritative sync lifecycle metadata.
// Session and thread snapshot bodies stay in the short-lived Redis cache.
type CollaborationSyncRequest struct {
	ent.Schema
}

func (CollaborationSyncRequest) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "collaboration_sync_requests"},
	}
}

func (CollaborationSyncRequest) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CollaborationSyncRequest) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.UUID("device_id", uuid.UUID{}).
			Immutable(),
		field.UUID("idempotency_key", uuid.UUID{}).
			Immutable(),
		field.Enum("kind").
			Values("session_list", "thread_snapshot").
			Immutable(),
		field.String("thread_id").
			MaxLen(512).
			Optional().
			Nillable().
			Immutable(),
		field.String("cursor").
			MaxLen(1024).
			Optional().
			Nillable().
			Immutable(),
		field.Enum("status").
			Values("pending", "running", "completed", "failed", "expired").
			Default("pending"),
		field.String("error_code").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Int64("snapshot_version").
			Optional().
			Nillable(),
		field.Int("result_count").
			Default(0),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CollaborationSyncRequest) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("collaboration_sync_requests").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		edge.From("device", CollaborationDevice.Type).
			Ref("sync_requests").
			Field("device_id").
			Unique().
			Required().
			Immutable(),
	}
}

func (CollaborationSyncRequest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "idempotency_key").Unique(),
		index.Fields("user_id", "device_id", "created_at"),
		index.Fields("user_id", "id"),
		index.Fields("status", "expires_at"),
	}
}
