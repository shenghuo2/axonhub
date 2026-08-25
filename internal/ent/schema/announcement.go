package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"

	"github.com/looplj/axonhub/internal/ent/schema/schematype"
	"github.com/looplj/axonhub/internal/scopes"
)

// Announcement is a message delivered to the owners of selected API keys.
type Announcement struct {
	ent.Schema
}

func (Announcement) Mixin() []ent.Mixin {
	return []ent.Mixin{TimeMixin{}, schematype.SoftDeleteMixin{}}
}

func (Announcement) Fields() []ent.Field {
	return []ent.Field{
		field.String("content").NotEmpty(),
	}
}

func (Announcement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type).
			Annotations(entgql.Skip(entgql.SkipAll)),
	}
}

func (Announcement) Annotations() []schema.Annotation {
	// Announcement delivery is exposed through the scoped custom GraphQL API,
	// never through the generic Ent query surface.
	return []schema.Annotation{entgql.Skip(entgql.SkipAll)}
}

func (Announcement) Policy() ent.Policy {
	return scopes.Policy{
		Query:    scopes.QueryPolicy{scopes.OwnerRule()},
		Mutation: scopes.MutationPolicy{scopes.OwnerRule()},
	}
}
