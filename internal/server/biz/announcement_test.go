package biz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/enttest"
)

func setupAnnouncementService(t *testing.T) (*AnnouncementService, *ent.Client, context.Context) {
	t.Helper()

	client := enttest.NewEntClient(t, "sqlite3", "file:announcement?mode=memory&_fk=1")
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))

	return &AnnouncementService{AbstractService: &AbstractService{db: client}}, client, ctx
}

func TestAnnouncementService_TargetsOnlySelectedAPIKeyOwners(t *testing.T) {
	service, client, ctx := setupAnnouncementService(t)
	defer client.Close()

	owner, err := client.User.Create().
		SetEmail("owner@example.com").
		SetPassword("password").
		SetIsOwner(true).
		Save(ctx)
	require.NoError(t, err)

	recipient, err := client.User.Create().
		SetEmail("recipient@example.com").
		SetPassword("password").
		Save(ctx)
	require.NoError(t, err)

	unrelatedUser, err := client.User.Create().
		SetEmail("unrelated@example.com").
		SetPassword("password").
		Save(ctx)
	require.NoError(t, err)

	project, err := client.Project.Create().
		SetName("announcement-test").
		Save(ctx)
	require.NoError(t, err)

	recipientKey, err := client.APIKey.Create().
		SetKey("ah-recipient").
		SetName("recipient-key").
		SetProject(project).
		SetUser(recipient).
		Save(ctx)
	require.NoError(t, err)

	secondRecipientKey, err := client.APIKey.Create().
		SetKey("ah-recipient-second").
		SetName("recipient-second-key").
		SetProject(project).
		SetUser(recipient).
		Save(ctx)
	require.NoError(t, err)

	unrelatedKey, err := client.APIKey.Create().
		SetKey("ah-unrelated").
		SetName("unrelated-key").
		SetProject(project).
		SetUser(unrelatedUser).
		Save(ctx)
	require.NoError(t, err)

	ownerCtx := contexts.WithUser(context.Background(), owner)
	created, err := service.Create(ownerCtx, CreateAnnouncementInput{
		Content:   "Targeted maintenance notice",
		APIKeyIDs: []int{recipientKey.ID, secondRecipientKey.ID, recipientKey.ID},
	})
	require.NoError(t, err)
	require.Equal(t, 2, created.RecipientCount)

	adminAnnouncements, err := service.List(ownerCtx)
	require.NoError(t, err)
	require.Len(t, adminAnnouncements, 1)
	require.Equal(t, 2, adminAnnouncements[0].RecipientCount)

	recipientAnnouncements, err := service.ListForCurrentUser(contexts.WithUser(context.Background(), recipient))
	require.NoError(t, err)
	require.Len(t, recipientAnnouncements, 1)
	require.Equal(t, created.Content, recipientAnnouncements[0].Content)

	unrelatedAnnouncements, err := service.ListForCurrentUser(contexts.WithUser(context.Background(), unrelatedUser))
	require.NoError(t, err)
	require.Empty(t, unrelatedAnnouncements)

	require.NoError(t, service.Delete(ownerCtx, created.ID.ID))

	adminAnnouncements, err = service.List(ownerCtx)
	require.NoError(t, err)
	require.Empty(t, adminAnnouncements)

	recipientAnnouncements, err = service.ListForCurrentUser(contexts.WithUser(context.Background(), recipient))
	require.NoError(t, err)
	require.Empty(t, recipientAnnouncements)

	_, err = service.Create(ownerCtx, CreateAnnouncementInput{Content: "No recipients"})
	require.ErrorContains(t, err, "select at least one API key")

	_, err = service.Create(ownerCtx, CreateAnnouncementInput{
		Content:   "Unrelated recipient is valid",
		APIKeyIDs: []int{unrelatedKey.ID},
	})
	require.NoError(t, err)

	archivedKey, err := client.APIKey.Create().
		SetKey("ah-archived").
		SetName("archived-key").
		SetProject(project).
		SetUser(recipient).
		SetStatus(apikey.StatusArchived).
		Save(ctx)
	require.NoError(t, err)

	_, err = service.Create(ownerCtx, CreateAnnouncementInput{
		Content:   "Archived recipient is invalid",
		APIKeyIDs: []int{archivedKey.ID},
	})
	require.ErrorContains(t, err, "one or more selected API keys")

	_, err = service.List(contexts.WithUser(context.Background(), recipient))
	require.ErrorContains(t, err, "read_users is required")
}
