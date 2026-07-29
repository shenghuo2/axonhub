package openapi

import (
	"github.com/99designs/gqlgen/graphql"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/server/biz"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	client                       *ent.Client
	apiKeyService                *biz.APIKeyService
	apiKeyProfileTemplateService *biz.APIKeyProfileTemplateService
	quotaService                 *biz.QuotaService
}

func NewSchema(
	client *ent.Client,
	apiKeyService *biz.APIKeyService,
	apiKeyProfileTemplateService *biz.APIKeyProfileTemplateService,
	quotaService *biz.QuotaService,
) graphql.ExecutableSchema {
	return NewExecutableSchema(Config{
		Resolvers: &Resolver{
			client:                       client,
			apiKeyService:                apiKeyService,
			apiKeyProfileTemplateService: apiKeyProfileTemplateService,
			quotaService:                 quotaService,
		},
	})
}
