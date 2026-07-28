package gql

import (
	"context"
	"fmt"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestServiceTierGraphQLRequestFilterAndPriceRoundTrip(t *testing.T) {
	db := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer db.Close()

	ctx := authz.WithTestBypass(context.Background())
	_, err := db.Request.Create().
		SetProjectID(1).
		SetSource(request.SourceAPI).
		SetModelID("gpt-5.4").
		SetServiceTier("fast").
		SetFormat("openai/chat_completions").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		Save(ctx)
	require.NoError(t, err)

	_, err = db.Request.Create().
		SetProjectID(1).
		SetSource(request.SourceAPI).
		SetModelID("gpt-5.4").
		SetFormat("openai/chat_completions").
		SetRequestBody(objects.JSONRawMessage(`{}`)).
		SetStatus(request.StatusCompleted).
		Save(ctx)
	require.NoError(t, err)

	ch, err := db.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Service tier price").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gpt-5.4"}).
		SetDefaultTestModel("gpt-5.4").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService := biz.NewChannelServiceForTest(db)
	defer channelService.Stop()
	handler := NewGraphqlHandlers(Dependencies{
		Ent:            db,
		ChannelService: channelService,
	})
	graphqlClient := client.New(handler.Graphql, func(req *client.Request) {
		req.HTTP = req.HTTP.WithContext(authz.WithTestBypass(req.HTTP.Context()))
	})

	var requestResponse struct {
		Requests struct {
			Edges []struct {
				Node struct {
					ServiceTier *string
				}
			}
			TotalCount int
		}
	}
	err = graphqlClient.Post(`
		query ServiceTierRequests {
			requests(first: 10, where: {serviceTier: "fast"}) {
				edges {
					node {
						serviceTier
					}
				}
				totalCount
			}
		}
	`, &requestResponse)
	require.NoError(t, err)
	require.Equal(t, 1, requestResponse.Requests.TotalCount)
	require.Len(t, requestResponse.Requests.Edges, 1)
	require.NotNil(t, requestResponse.Requests.Edges[0].Node.ServiceTier)
	require.Equal(t, "fast", *requestResponse.Requests.Edges[0].Node.ServiceTier)

	priceInput := []map[string]any{
		{
			"modelId": "gpt-5.4",
			"price": map[string]any{
				"items": []map[string]any{
					{
						"itemCode": "prompt_tokens",
						"pricing": map[string]any{
							"mode":         "usage_per_unit",
							"usagePerUnit": "1",
						},
					},
				},
				"serviceTierMultipliers": []map[string]any{
					{"serviceTier": "fast", "multiplier": "2.5"},
				},
			},
		},
	}
	channelID := fmt.Sprintf("gid://axonhub/%s/%d", ent.TypeChannel, ch.ID)

	var mutationResponse struct {
		SaveChannelModelPrices []struct {
			Price struct {
				ServiceTierMultipliers []struct {
					ServiceTier string
					Multiplier  float64
				}
			}
		}
	}
	err = graphqlClient.Post(`
		mutation SaveServiceTierPrice($channelId: ID!, $input: [SaveChannelModelPriceInput!]!) {
			saveChannelModelPrices(channelId: $channelId, input: $input) {
				price {
					serviceTierMultipliers {
						serviceTier
						multiplier
					}
				}
			}
		}
	`, &mutationResponse, client.Var("channelId", channelID), client.Var("input", priceInput))
	require.NoError(t, err)
	require.Len(t, mutationResponse.SaveChannelModelPrices, 1)
	require.Len(t, mutationResponse.SaveChannelModelPrices[0].Price.ServiceTierMultipliers, 1)
	require.Equal(t, "fast", mutationResponse.SaveChannelModelPrices[0].Price.ServiceTierMultipliers[0].ServiceTier)
	require.Equal(t, 2.5, mutationResponse.SaveChannelModelPrices[0].Price.ServiceTierMultipliers[0].Multiplier)

	var queryResponse struct {
		Node struct {
			ChannelModelPrices []struct {
				Price struct {
					ServiceTierMultipliers []struct {
						ServiceTier string
						Multiplier  float64
					}
				}
			}
		}
	}
	err = graphqlClient.Post(`
		query ServiceTierPrice($id: ID!) {
			node(id: $id) {
				... on Channel {
					channelModelPrices {
						price {
							serviceTierMultipliers {
								serviceTier
								multiplier
							}
						}
					}
				}
			}
		}
	`, &queryResponse, client.Var("id", channelID))
	require.NoError(t, err)
	require.Len(t, queryResponse.Node.ChannelModelPrices, 1)
	require.Len(t, queryResponse.Node.ChannelModelPrices[0].Price.ServiceTierMultipliers, 1)
	require.Equal(t, "fast", queryResponse.Node.ChannelModelPrices[0].Price.ServiceTierMultipliers[0].ServiceTier)
	require.Equal(t, 2.5, queryResponse.Node.ChannelModelPrices[0].Price.ServiceTierMultipliers[0].Multiplier)
}
