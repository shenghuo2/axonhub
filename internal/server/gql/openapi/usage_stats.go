package openapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/usagelog"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/scopes"
)

const (
	defaultProjectAPIKeyPageSize = 100
	maxProjectAPIKeyPageSize     = 100
	maxUsageStatsAPIKeys         = 100
	maxJSONSafeInteger     int64 = 1<<53 - 1
)

type usageStatsRow struct {
	APIKeyID        int     `json:"api_key_id"`
	ModelID         string  `json:"model_id"`
	ServiceTier     *string `json:"service_tier"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
}

func (r *Resolver) resolveProjectAPIKey(
	ctx context.Context,
	id *objects.GUID,
	key *string,
	name *string,
) (*ProjectAPIKey, error) {
	if err := authz.RequireScope(ctx, scopes.ScopeReadUsageStats); err != nil {
		return nil, err
	}

	keyID, err := guidID(id, ent.TypeAPIKey)
	if err != nil {
		return nil, err
	}
	if selectorCount(keyID != nil, key != nil, name != nil) != 1 {
		return nil, fmt.Errorf("provide exactly one of id, key, or name")
	}

	principal, ok := contexts.GetAPIKey(ctx)
	if !ok || principal == nil {
		return nil, fmt.Errorf("api key not found in context")
	}

	query := r.client.APIKey.Query().Where(
		apikey.ProjectIDEQ(principal.ProjectID),
		apikey.TypeNEQ(apikey.TypePersonal),
	)
	switch {
	case keyID != nil:
		query = query.Where(apikey.IDEQ(*keyID))
	case key != nil:
		query = query.Where(apikey.KeyEQ(*key))
	case name != nil:
		query = query.Where(apikey.NameEQ(strings.TrimSpace(*name)))
	}

	item, err := query.Only(authz.WithScopeDecision(ctx, scopes.ScopeReadUsageStats))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("API key not found in service account project")
		}
		return nil, fmt.Errorf("query project API key: %w", err)
	}

	return toProjectAPIKey(item), nil
}

func (r *Resolver) listProjectAPIKeys(
	ctx context.Context,
	first *int,
	after *string,
) (*ProjectAPIKeyConnection, error) {
	if err := authz.RequireScope(ctx, scopes.ScopeReadUsageStats); err != nil {
		return nil, err
	}

	pageSize := defaultProjectAPIKeyPageSize
	if first != nil {
		pageSize = *first
	}
	if pageSize < 1 || pageSize > maxProjectAPIKeyPageSize {
		return nil, fmt.Errorf("first must be between 1 and %d", maxProjectAPIKeyPageSize)
	}

	afterID, err := decodeProjectAPIKeyCursor(after)
	if err != nil {
		return nil, err
	}
	principal, ok := contexts.GetAPIKey(ctx)
	if !ok || principal == nil {
		return nil, fmt.Errorf("api key not found in context")
	}

	query := r.client.APIKey.Query().Where(
		apikey.ProjectIDEQ(principal.ProjectID),
		apikey.TypeNEQ(apikey.TypePersonal),
	)
	if afterID > 0 {
		query = query.Where(apikey.IDGT(afterID))
	}

	items, err := query.
		Order(ent.Asc(apikey.FieldID)).
		Limit(pageSize + 1).
		All(authz.WithScopeDecision(ctx, scopes.ScopeReadUsageStats))
	if err != nil {
		return nil, fmt.Errorf("list project API keys: %w", err)
	}

	hasNextPage := len(items) > pageSize
	if hasNextPage {
		items = items[:pageSize]
	}
	edges := make([]*ProjectAPIKeyEdge, 0, len(items))
	for _, item := range items {
		edges = append(edges, &ProjectAPIKeyEdge{
			Cursor: encodeProjectAPIKeyCursor(item.ID),
			Node:   toProjectAPIKey(item),
		})
	}

	var endCursor *string
	if len(edges) > 0 {
		cursor := edges[len(edges)-1].Cursor
		endCursor = &cursor
	}

	return &ProjectAPIKeyConnection{
		Edges: edges,
		PageInfo: &ProjectAPIKeyPageInfo{
			HasNextPage: hasNextPage,
			EndCursor:   endCursor,
		},
	}, nil
}

func (r *Resolver) queryAPIKeyTokenUsageStats(
	ctx context.Context,
	input APIKeyTokenUsageStatsInput,
) ([]*APIKeyTokenUsageStats, error) {
	if err := authz.RequireScope(ctx, scopes.ScopeReadUsageStats); err != nil {
		return nil, err
	}
	if len(input.APIKeyIds) == 0 {
		return nil, fmt.Errorf("apiKeyIds is required and must contain at least one API key")
	}
	if len(input.APIKeyIds) > maxUsageStatsAPIKeys {
		return nil, fmt.Errorf("apiKeyIds cannot exceed %d items", maxUsageStatsAPIKeys)
	}
	if input.CreatedAtGte != nil && input.CreatedAtLte != nil && input.CreatedAtGte.After(*input.CreatedAtLte) {
		return nil, fmt.Errorf("createdAtGTE must not be after createdAtLTE")
	}

	requestedIDs := make([]int, 0, len(input.APIKeyIds))
	seen := make(map[int]struct{}, len(input.APIKeyIds))
	for _, guid := range input.APIKeyIds {
		if guid == nil || guid.Type != ent.TypeAPIKey {
			return nil, fmt.Errorf("apiKeyIds must contain only APIKey IDs")
		}
		if _, exists := seen[guid.ID]; exists {
			continue
		}
		seen[guid.ID] = struct{}{}
		requestedIDs = append(requestedIDs, guid.ID)
	}

	principal, ok := contexts.GetAPIKey(ctx)
	if !ok || principal == nil {
		return nil, fmt.Errorf("api key not found in context")
	}
	statsCtx := authz.WithScopeDecision(ctx, scopes.ScopeReadUsageStats)
	accessibleIDs, err := r.client.APIKey.Query().
		Where(
			apikey.IDIn(requestedIDs...),
			apikey.ProjectIDEQ(principal.ProjectID),
			apikey.TypeNEQ(apikey.TypePersonal),
		).
		IDs(statsCtx)
	if err != nil {
		return nil, fmt.Errorf("validate project API key access: %w", err)
	}
	if len(accessibleIDs) == 0 {
		return []*APIKeyTokenUsageStats{}, nil
	}

	query := r.client.UsageLog.Query().Where(
		usagelog.ProjectIDEQ(principal.ProjectID),
		usagelog.APIKeyIDIn(accessibleIDs...),
	)
	if input.CreatedAtGte != nil {
		query = query.Where(usagelog.CreatedAtGTE(*input.CreatedAtGte))
	}
	if input.CreatedAtLte != nil {
		query = query.Where(usagelog.CreatedAtLTE(*input.CreatedAtLte))
	}

	var rows []usageStatsRow
	err = query.Modify(func(s *sql.Selector) {
		requestTable := sql.Table(request.Table)
		s.Join(requestTable).On(
			s.C(usagelog.FieldRequestID),
			requestTable.C(request.FieldID),
		)
		s.Select(
			s.C(usagelog.FieldAPIKeyID),
			s.C(usagelog.FieldModelID),
			sql.As(requestTable.C(request.FieldServiceTier), "service_tier"),
			sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptTokens)), "input_tokens"),
			sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionTokens)), "output_tokens"),
			sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldPromptCachedTokens)), "cached_tokens"),
			sql.As(fmt.Sprintf("COALESCE(SUM(%s), 0)", s.C(usagelog.FieldCompletionReasoningTokens)), "reasoning_tokens"),
		).GroupBy(
			s.C(usagelog.FieldAPIKeyID),
			s.C(usagelog.FieldModelID),
			requestTable.C(request.FieldServiceTier),
		)
	}).Scan(statsCtx, &rows)
	if err != nil {
		return nil, fmt.Errorf("query API key token usage stats: %w", err)
	}

	return buildTokenUsageStats(accessibleIDs, rows), nil
}

func buildTokenUsageStats(accessibleIDs []int, rows []usageStatsRow) []*APIKeyTokenUsageStats {
	type detailAccumulator struct {
		modelID         string
		serviceTier     *string
		inputTokens     int64
		outputTokens    int64
		cachedTokens    int64
		reasoningTokens int64
	}
	type keyAccumulator struct {
		inputTokens     int64
		outputTokens    int64
		cachedTokens    int64
		reasoningTokens int64
		details         map[string]*detailAccumulator
	}

	byKey := make(map[int]*keyAccumulator, len(accessibleIDs))
	for _, id := range accessibleIDs {
		byKey[id] = &keyAccumulator{details: make(map[string]*detailAccumulator)}
	}
	for _, row := range rows {
		keyStats, ok := byKey[row.APIKeyID]
		if !ok {
			continue
		}
		tier := normalizeServiceTier(row.ServiceTier)
		tierKey := ""
		if tier != nil {
			tierKey = *tier
		}
		detailKey := row.ModelID + "\x00" + tierKey
		detail, exists := keyStats.details[detailKey]
		if !exists {
			detail = &detailAccumulator{modelID: row.ModelID, serviceTier: tier}
			keyStats.details[detailKey] = detail
		}

		detail.inputTokens += row.InputTokens
		detail.outputTokens += row.OutputTokens
		detail.cachedTokens += row.CachedTokens
		detail.reasoningTokens += row.ReasoningTokens
		keyStats.inputTokens += row.InputTokens
		keyStats.outputTokens += row.OutputTokens
		keyStats.cachedTokens += row.CachedTokens
		keyStats.reasoningTokens += row.ReasoningTokens
	}

	sort.Ints(accessibleIDs)
	result := make([]*APIKeyTokenUsageStats, 0, len(accessibleIDs))
	for _, id := range accessibleIDs {
		item := byKey[id]
		details := make([]*ModelServiceTierTokenUsageStats, 0, len(item.details))
		for _, detail := range item.details {
			details = append(details, &ModelServiceTierTokenUsageStats{
				ModelID:         detail.modelID,
				ServiceTier:     detail.serviceTier,
				InputTokens:     usageInt(detail.inputTokens),
				OutputTokens:    usageInt(detail.outputTokens),
				CachedTokens:    usageInt(detail.cachedTokens),
				ReasoningTokens: usageInt(detail.reasoningTokens),
			})
		}
		sort.Slice(details, func(i, j int) bool {
			if details[i].ModelID != details[j].ModelID {
				return details[i].ModelID < details[j].ModelID
			}
			return serviceTierSortKey(details[i].ServiceTier) < serviceTierSortKey(details[j].ServiceTier)
		})

		result = append(result, &APIKeyTokenUsageStats{
			APIKeyID:               objects.GUID{Type: ent.TypeAPIKey, ID: id},
			InputTokens:            usageInt(item.inputTokens),
			OutputTokens:           usageInt(item.outputTokens),
			CachedTokens:           usageInt(item.cachedTokens),
			ReasoningTokens:        usageInt(item.reasoningTokens),
			ModelServiceTierUsages: details,
		})
	}
	return result
}

func toProjectAPIKey(item *ent.APIKey) *ProjectAPIKey {
	return &ProjectAPIKey{
		ID:     objects.GUID{Type: ent.TypeAPIKey, ID: item.ID},
		Name:   item.Name,
		Type:   item.Type.String(),
		Status: item.Status.String(),
	}
}

func selectorCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}

func encodeProjectAPIKeyCursor(id int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(id)))
}

func decodeProjectAPIKeyCursor(cursor *string) (int, error) {
	if cursor == nil || *cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(*cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid API key cursor")
	}
	id, err := strconv.Atoi(string(decoded))
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid API key cursor")
	}
	return id, nil
}

func normalizeServiceTier(value *string) *string {
	if value == nil {
		return nil
	}
	tier := strings.ToLower(strings.TrimSpace(*value))
	if tier == "" {
		return nil
	}
	return &tier
}

func serviceTierSortKey(value *string) string {
	if value == nil {
		return ""
	}
	return "1" + *value
}

func usageInt(value int64) int {
	if value <= 0 {
		return 0
	}
	if value > maxJSONSafeInteger {
		return int(maxJSONSafeInteger)
	}
	return int(value)
}
