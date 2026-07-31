package openapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/project"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/scopes"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/internal/server/gql/openapi"
	"github.com/looplj/axonhub/internal/server/middleware"
)

// e2eEnv bundles a real httptest server wired with the production OpenAPI route
// stack (WithEntClient -> WithOpenAPIAuth -> the gqlgen handler) plus the seeded
// keys used by the e2e cases.
type e2eEnv struct {
	server     *httptest.Server
	saKey               string // service_account with read_api_keys
	saUsageKey          string // service_account with read_usage_stats only
	saUsageAggregateKey string // service_account with read_usage_aggregates only
	saNoScope           string // service_account without usage read scopes
	targetID   int    // user key with a quota profile (same project)
	targetKey  string
	personalID int
	foreignID  int // user key in a different project
	foreignKey string
}

const quotaQuery = `query($id: ID, $key: String) {
  apiKeyQuotaUsages(apiKeyId: $id, key: $key) {
    profileName
    window { start end }
    usage { requestCount totalTokens totalCost }
  }
}`

// gqlResponse mirrors the wire JSON of an apiKeyQuotaUsages response.
type gqlResponse struct {
	Data struct {
		APIKeyQuotaUsages []struct {
			ProfileName string `json:"profileName"`
			Window      struct {
				Start *string `json:"start"`
				End   *string `json:"end"`
			} `json:"window"`
			Usage struct {
				RequestCount int             `json:"requestCount"`
				TotalTokens  int             `json:"totalTokens"`
				TotalCost    json.RawMessage `json:"totalCost"`
			} `json:"usage"`
		} `json:"apiKeyQuotaUsages"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func setupE2E(t *testing.T) e2eEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	client := enttest.NewEntClient(t, "sqlite3", "file:ent_e2e?mode=memory&_fk=1")
	t.Cleanup(func() { _ = client.Close() })

	// Seed with a privacy bypass; build all entities BEFORE the services so the
	// api key cache picks them up on initial load.
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	now := time.Now().UTC()

	hashed, err := biz.HashPassword("pw")
	require.NoError(t, err)

	owner := client.User.Create().
		SetEmail(fmt.Sprintf("owner-%d@example.com", now.UnixNano())).
		SetPassword(hashed).SetFirstName("O").SetLastName("U").
		SetStatus(user.StatusActivated).SaveX(ctx)

	proj := client.Project.Create().
		SetName(fmt.Sprintf("proj-%d", now.UnixNano())).
		SetStatus(project.StatusActive).SaveX(ctx)

	otherProj := client.Project.Create().
		SetName(fmt.Sprintf("other-%d", now.UnixNano())).
		SetStatus(project.StatusActive).SaveX(ctx)

	mustKey := func(name string, projID int, typ apikey.Type, scopeList []string, profiles *objects.APIKeyProfiles) *ent.APIKey {
		val, err := biz.GenerateAPIKey("ah")
		require.NoError(t, err)
		b := client.APIKey.Create().
			SetName(name).SetKey(val).SetUserID(owner.ID).SetProjectID(projID).
			SetType(typ).SetStatus(apikey.StatusEnabled).SetScopes(scopeList)
		if profiles != nil {
			b = b.SetProfiles(profiles)
		}
		return b.SaveX(ctx)
	}

	sa := mustKey("sa", proj.ID, apikey.TypeServiceAccount, []string{string(scopes.ScopeReadAPIKeys)}, nil)
	saUsage := mustKey("sa-usage", proj.ID, apikey.TypeServiceAccount, []string{string(scopes.ScopeReadUsageStats)}, nil)
	saUsageAggregate := mustKey("sa-usage-aggregate", proj.ID, apikey.TypeServiceAccount, []string{string(scopes.ScopeReadUsageAggregates)}, nil)
	saNoScope := mustKey("sa-noscope", proj.ID, apikey.TypeServiceAccount, []string{string(scopes.ScopeWriteAPIKeys)}, nil)

	quotaProfile := &objects.APIKeyProfiles{
		ActiveProfile: "Default",
		Profiles: []objects.APIKeyProfile{{
			Name: "Default",
			Quota: &objects.APIKeyQuota{
				Requests: ptr(int64(1000)),
				Cost:     ptrDecimalFromInt(100),
				Period:   objects.APIKeyQuotaPeriod{Type: objects.APIKeyQuotaPeriodTypeAllTime},
			},
		}},
	}
	target := mustKey("target", proj.ID, apikey.TypeUser, nil, quotaProfile)
	personal := mustKey("personal", proj.ID, apikey.TypePersonal, nil, quotaProfile)
	foreign := mustKey("foreign", otherProj.ID, apikey.TypeUser, nil, quotaProfile)

	// Two usage rows for the target key → requestCount=2, totalTokens=300, totalCost=2.
	for i := range 2 {
		modelID := "gpt-5.4"
		serviceTier := ""
		if i == 0 {
			modelID = "gpt-5.6"
			serviceTier = "FAST"
		}
		req := client.Request.Create().
			SetProjectID(proj.ID).SetAPIKeyID(target.ID).SetModelID(modelID).
			SetFormat("openai/chat_completions").SetStatus(request.StatusCompleted).
			SetRequestBody(objects.JSONRawMessage([]byte(`{}`))).
			SetCreatedAt(now.Add(-time.Duration(i+1) * time.Minute))
		if serviceTier != "" {
			req = req.SetServiceTier(serviceTier)
		}
		requestRow := req.SaveX(ctx)

		client.UsageLog.Create().
			SetRequestID(requestRow.ID).SetAPIKeyID(target.ID).SetProjectID(proj.ID).
			SetChannelID(1).SetModelID(modelID).SetSource(usagelog.SourceAPI).
			SetFormat("openai/chat_completions").
			SetPromptTokens(50).SetCompletionTokens(100).SetTotalTokens(150).
			SetPromptCachedTokens(10).SetCompletionReasoningTokens(20).
			SetTotalCost(1.0).
			SetCreatedAt(now.Add(-time.Duration(i+1) * time.Minute)).SaveX(ctx)
	}

	// Real services (memory cache, no Redis).
	cacheCfg := xcache.Config{Mode: xcache.ModeMemory}
	projectSvc := &biz.ProjectService{
		ProjectCache: xcache.NewFromConfig[xcache.Entry[ent.Project]](cacheCfg),
	}
	apiKeySvc := biz.NewAPIKeyService(biz.APIKeyServiceParams{
		CacheConfig: cacheCfg, Ent: client, ProjectService: projectSvc, KeyPrefix: "ah",
	})
	t.Cleanup(apiKeySvc.Stop)
	tmplSvc := biz.NewAPIKeyProfileTemplateService(biz.APIKeyProfileTemplateServiceParams{Ent: client})
	systemSvc := biz.NewSystemService(biz.SystemServiceParams{Ent: client})
	quotaSvc := biz.NewQuotaService(client, systemSvc)
	authSvc := biz.NewAuthService(biz.AuthServiceParams{
		SystemService: systemSvc, APIKeyService: apiKeySvc, Ent: client,
	})

	handler := openapi.NewGraphqlHandlers(openapi.Dependencies{
		Ent: client, APIKeyService: apiKeySvc, APIKeyProfileTemplateService: tmplSvc, QuotaService: quotaSvc,
	})

	// Reproduce the production route stack from routes.go.
	engine := gin.New()
	engine.Use(middleware.WithEntClient(client))
	grp := engine.Group("/openapi", middleware.WithOpenAPIAuth(authSvc), middleware.WithTimeout(30*time.Second))
	grp.POST("/v1/graphql", func(c *gin.Context) {
		handler.Graphql.ServeHTTP(c.Writer, c.Request)
	})

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	return e2eEnv{
		server: srv,
		saKey: sa.Key, saUsageKey: saUsage.Key, saUsageAggregateKey: saUsageAggregate.Key, saNoScope: saNoScope.Key,
		targetID: target.ID, targetKey: target.Key,
		personalID: personal.ID,
		foreignID: foreign.ID, foreignKey: foreign.Key,
	}
}

func gqlPost(t *testing.T, url, bearer string, vars map[string]any) (int, []byte) {
	return gqlPostQuery(t, url, bearer, quotaQuery, vars)
}

func gqlPostQuery(t *testing.T, url, bearer, query string, vars map[string]any) (int, []byte) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url+"/openapi/v1/graphql", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body
}

func TestE2E_ProjectUsageStats_ServiceAccountScope(t *testing.T) {
	env := setupE2E(t)
	targetGUID := fmt.Sprintf("gid://axonhub/APIKey/%d", env.targetID)
	foreignGUID := fmt.Sprintf("gid://axonhub/APIKey/%d", env.foreignID)
	personalGUID := fmt.Sprintf("gid://axonhub/APIKey/%d", env.personalID)

	t.Run("safe metadata works without read_api_keys", func(t *testing.T) {
		query := `query($id: ID!) {
		  projectAPIKey(id: $id) { id name type status }
		}`
		code, body := gqlPostQuery(t, env.server.URL, env.saUsageKey, query, map[string]any{"id": targetGUID})
		require.Equal(t, http.StatusOK, code)

		var response struct {
			Data struct {
				ProjectAPIKey struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"projectAPIKey"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.Empty(t, response.Errors)
		require.Equal(t, targetGUID, response.Data.ProjectAPIKey.ID)
		require.Equal(t, "target", response.Data.ProjectAPIKey.Name)
		require.Equal(t, "user", response.Data.ProjectAPIKey.Type)
	})

	t.Run("list is paginated and hides personal keys", func(t *testing.T) {
		query := `query($after: String) {
		  projectAPIKeys(first: 2, after: $after) {
		    edges { cursor node { id name type status } }
		    pageInfo { hasNextPage endCursor }
		  }
		}`
		after := any(nil)
		names := make([]string, 0)
		for {
			code, body := gqlPostQuery(t, env.server.URL, env.saUsageKey, query, map[string]any{"after": after})
			require.Equal(t, http.StatusOK, code)
			var response struct {
				Data struct {
					ProjectAPIKeys struct {
						Edges []struct {
							Cursor string `json:"cursor"`
							Node   struct {
								Name string `json:"name"`
							} `json:"node"`
						} `json:"edges"`
						PageInfo struct {
							HasNextPage bool    `json:"hasNextPage"`
							EndCursor   *string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"projectAPIKeys"`
				} `json:"data"`
				Errors []any `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(body, &response))
			require.Empty(t, response.Errors)
			for _, edge := range response.Data.ProjectAPIKeys.Edges {
				names = append(names, edge.Node.Name)
			}
			if !response.Data.ProjectAPIKeys.PageInfo.HasNextPage {
				break
			}
			require.NotNil(t, response.Data.ProjectAPIKeys.PageInfo.EndCursor)
			after = *response.Data.ProjectAPIKeys.PageInfo.EndCursor
		}
		require.Contains(t, names, "target")
		require.NotContains(t, names, "personal")
		require.NotContains(t, names, "foreign")
	})

	t.Run("usage is grouped by model and normalized service tier", func(t *testing.T) {
		query := `query($input: APIKeyTokenUsageStatsInput!) {
		  apiKeyTokenUsageStats(input: $input) {
		    apiKeyId inputTokens outputTokens cachedTokens reasoningTokens
		    modelServiceTierUsages {
		      modelId serviceTier inputTokens outputTokens cachedTokens reasoningTokens
		    }
		  }
		}`
		code, body := gqlPostQuery(t, env.server.URL, env.saUsageKey, query, map[string]any{
			"input": map[string]any{"apiKeyIds": []string{targetGUID, foreignGUID, personalGUID}},
		})
		require.Equal(t, http.StatusOK, code)
		var response struct {
			Data struct {
				Stats []struct {
					APIKeyID        string `json:"apiKeyId"`
					InputTokens     int    `json:"inputTokens"`
					OutputTokens    int    `json:"outputTokens"`
					CachedTokens    int    `json:"cachedTokens"`
					ReasoningTokens int    `json:"reasoningTokens"`
					Details         []struct {
						ModelID     string  `json:"modelId"`
						ServiceTier *string `json:"serviceTier"`
						InputTokens int     `json:"inputTokens"`
					} `json:"modelServiceTierUsages"`
				} `json:"apiKeyTokenUsageStats"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.Empty(t, response.Errors)
		require.Len(t, response.Data.Stats, 1)
		stats := response.Data.Stats[0]
		require.Equal(t, targetGUID, stats.APIKeyID)
		require.Equal(t, 100, stats.InputTokens)
		require.Equal(t, 200, stats.OutputTokens)
		require.Equal(t, 20, stats.CachedTokens)
		require.Equal(t, 40, stats.ReasoningTokens)
		require.Len(t, stats.Details, 2)
		require.Equal(t, "gpt-5.4", stats.Details[0].ModelID)
		require.Nil(t, stats.Details[0].ServiceTier)
		require.Equal(t, "gpt-5.6", stats.Details[1].ModelID)
		require.Equal(t, "fast", *stats.Details[1].ServiceTier)
	})

	t.Run("usage log aggregate is project-scoped and does not require request access", func(t *testing.T) {
		query := `query($input: APIKeyTokenUsageStatsInput!) {
		  projectAPIKeys(first: 10) { edges { node { id name } } }
		  apiKeyUsageLogAggregates(input: $input) {
		    apiKeyId inputTokens outputTokens cachedTokens reasoningTokens
		    modelUsages { modelId inputTokens outputTokens cachedTokens reasoningTokens }
		  }
		}`
		code, body := gqlPostQuery(t, env.server.URL, env.saUsageAggregateKey, query, map[string]any{
			"input": map[string]any{"apiKeyIds": []string{targetGUID, foreignGUID, personalGUID}},
		})
		require.Equal(t, http.StatusOK, code)
		var response struct {
			Data struct {
				ProjectAPIKeys struct {
					Edges []struct {
						Node struct {
							Name string `json:"name"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"projectAPIKeys"`
				Stats []struct {
					APIKeyID        string `json:"apiKeyId"`
					InputTokens     int    `json:"inputTokens"`
					OutputTokens    int    `json:"outputTokens"`
					CachedTokens    int    `json:"cachedTokens"`
					ReasoningTokens int    `json:"reasoningTokens"`
					Models          []struct {
						ModelID string `json:"modelId"`
					} `json:"modelUsages"`
				} `json:"apiKeyUsageLogAggregates"`
			} `json:"data"`
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.Empty(t, response.Errors)
		projectKeyNames := make([]string, 0, len(response.Data.ProjectAPIKeys.Edges))
		for _, edge := range response.Data.ProjectAPIKeys.Edges {
			projectKeyNames = append(projectKeyNames, edge.Node.Name)
		}
		require.Contains(t, projectKeyNames, "target")
		require.NotContains(t, projectKeyNames, "personal")
		require.Len(t, response.Data.Stats, 1)
		stats := response.Data.Stats[0]
		require.Equal(t, targetGUID, stats.APIKeyID)
		require.Equal(t, 100, stats.InputTokens)
		require.Equal(t, 200, stats.OutputTokens)
		require.Equal(t, 20, stats.CachedTokens)
		require.Equal(t, 40, stats.ReasoningTokens)
		require.Len(t, stats.Models, 2)
		require.Equal(t, "gpt-5.4", stats.Models[0].ModelID)
		require.Equal(t, "gpt-5.6", stats.Models[1].ModelID)
	})

	t.Run("usage aggregate scope cannot read service tier stats", func(t *testing.T) {
		query := `query($input: APIKeyTokenUsageStatsInput!) {
		  apiKeyTokenUsageStats(input: $input) { apiKeyId }
		}`
		code, body := gqlPostQuery(t, env.server.URL, env.saUsageAggregateKey, query, map[string]any{
			"input": map[string]any{"apiKeyIds": []string{targetGUID}},
		})
		require.Equal(t, http.StatusOK, code)
		var response struct {
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Errors)
	})

	t.Run("service tier scope cannot read usage log aggregates", func(t *testing.T) {
		query := `query($input: APIKeyTokenUsageStatsInput!) {
		  apiKeyUsageLogAggregates(input: $input) { apiKeyId }
		}`
		code, body := gqlPostQuery(t, env.server.URL, env.saUsageKey, query, map[string]any{
			"input": map[string]any{"apiKeyIds": []string{targetGUID}},
		})
		require.Equal(t, http.StatusOK, code)
		var response struct {
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Errors)
	})

	t.Run("missing usage scope is denied", func(t *testing.T) {
		query := `query { projectAPIKeys(first: 1) { edges { node { id } } } }`
		code, body := gqlPostQuery(t, env.server.URL, env.saNoScope, query, nil)
		require.Equal(t, http.StatusOK, code)
		var response struct {
			Errors []any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(body, &response))
		require.NotEmpty(t, response.Errors)
	})
}

func TestE2E_APIKeyQuotaUsages_FullStack(t *testing.T) {
	env := setupE2E(t)

	guid := fmt.Sprintf("gid://axonhub/APIKey/%d", env.targetID)

	t.Run("by apiKeyId returns seeded usage", func(t *testing.T) {
		code, body := gqlPost(t, env.server.URL, env.saKey, map[string]any{"id": guid})
		t.Logf("HTTP %d body: %s", code, body)
		require.Equal(t, http.StatusOK, code)

		var r gqlResponse
		require.NoError(t, json.Unmarshal(body, &r))
		require.Empty(t, r.Errors)
		require.Len(t, r.Data.APIKeyQuotaUsages, 1)
		u := r.Data.APIKeyQuotaUsages[0]
		require.Equal(t, "Default", u.ProfileName)
		require.Equal(t, 2, u.Usage.RequestCount)
		require.Equal(t, 300, u.Usage.TotalTokens)
		require.Equal(t, "2", string(u.Usage.TotalCost))
		// all_time window: open start, bounded end.
		require.Nil(t, u.Window.Start)
		require.NotNil(t, u.Window.End)
	})

	t.Run("by plaintext key returns seeded usage", func(t *testing.T) {
		code, body := gqlPost(t, env.server.URL, env.saKey, map[string]any{"key": env.targetKey})
		require.Equal(t, http.StatusOK, code)
		var r gqlResponse
		require.NoError(t, json.Unmarshal(body, &r))
		require.Empty(t, r.Errors)
		require.Len(t, r.Data.APIKeyQuotaUsages, 1)
		require.Equal(t, 2, r.Data.APIKeyQuotaUsages[0].Usage.RequestCount)
	})

	t.Run("cross-project key id is denied (graphql error, null data)", func(t *testing.T) {
		fguid := fmt.Sprintf("gid://axonhub/APIKey/%d", env.foreignID)
		code, body := gqlPost(t, env.server.URL, env.saKey, map[string]any{"id": fguid})
		t.Logf("cross-project HTTP %d body: %s", code, body)
		require.Equal(t, http.StatusOK, code)
		var r gqlResponse
		require.NoError(t, json.Unmarshal(body, &r))
		require.NotEmpty(t, r.Errors, "foreign key must surface an error")
		require.Nil(t, r.Data.APIKeyQuotaUsages)
	})

	t.Run("cross-project plaintext key is denied", func(t *testing.T) {
		code, body := gqlPost(t, env.server.URL, env.saKey, map[string]any{"key": env.foreignKey})
		require.Equal(t, http.StatusOK, code)
		var r gqlResponse
		require.NoError(t, json.Unmarshal(body, &r))
		require.NotEmpty(t, r.Errors)
	})

	t.Run("missing read_api_keys scope is denied", func(t *testing.T) {
		code, body := gqlPost(t, env.server.URL, env.saNoScope, map[string]any{"id": guid})
		t.Logf("missing-scope HTTP %d body: %s", code, body)
		require.Equal(t, http.StatusOK, code)
		var r gqlResponse
		require.NoError(t, json.Unmarshal(body, &r))
		require.NotEmpty(t, r.Errors)
	})

	t.Run("non-service-account bearer is rejected by auth (401)", func(t *testing.T) {
		// The target user-type key cannot use the OpenAPI surface at all.
		code, _ := gqlPost(t, env.server.URL, env.targetKey, map[string]any{"id": guid})
		require.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("missing bearer is rejected by auth (401)", func(t *testing.T) {
		code, _ := gqlPost(t, env.server.URL, "", map[string]any{"id": guid})
		require.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("GET on the graphql path is not routed (404) so a key cannot ride the URL", func(t *testing.T) {
		// Production registers only POST /v1/graphql; a GET never reaches the
		// handler. (The handler also rejects GET at the transport layer — see
		// TestOpenAPIHandler_RejectsGET — but gin closes the door first.)
		req, err := http.NewRequest(http.MethodGet,
			env.server.URL+"/openapi/v1/graphql?query=%7B__typename%7D&variables=%7B%22key%22%3A%22"+env.targetKey+"%22%7D", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+env.saKey)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func ptr[T any](v T) *T { return &v }

func ptrDecimalFromInt(v int64) *decimal.Decimal {
	d := decimal.NewFromInt(v)
	return &d
}
