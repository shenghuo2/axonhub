package biz

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/samber/lo"
	"go.uber.org/fx"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/announcement"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/scopes"
)

const maxAnnouncementContentLength = 2000

type AnnouncementServiceParams struct {
	fx.In

	Ent *ent.Client
}

type AnnouncementService struct {
	*AbstractService
}

type Announcement struct {
	ID             objects.GUID `json:"id"`
	Content        string       `json:"content"`
	CreatedAt      time.Time    `json:"createdAt"`
	RecipientCount int          `json:"recipientCount"`
}

type AnnouncementRecipientAPIKey struct {
	ID   objects.GUID `json:"id"`
	Name string       `json:"name"`
}

type CreateAnnouncementInput struct {
	Content   string
	APIKeyIDs []int
}

func NewAnnouncementService(params AnnouncementServiceParams) *AnnouncementService {
	return &AnnouncementService{
		AbstractService: &AbstractService{db: params.Ent},
	}
}

func (s *AnnouncementService) Create(ctx context.Context, input CreateAnnouncementInput) (*Announcement, error) {
	if err := requireAnnouncementScope(ctx, scopes.ScopeWriteUsers); err != nil {
		return nil, err
	}

	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, fmt.Errorf("announcement content is required")
	}
	if utf8.RuneCountInString(content) > maxAnnouncementContentLength {
		return nil, fmt.Errorf("announcement content must not exceed %d characters", maxAnnouncementContentLength)
	}

	apiKeyIDs := lo.Uniq(input.APIKeyIDs)
	if len(apiKeyIDs) == 0 {
		return nil, fmt.Errorf("select at least one API key")
	}
	if !lo.EveryBy(apiKeyIDs, func(id int) bool { return id > 0 }) {
		return nil, fmt.Errorf("invalid API key ID")
	}

	var created *ent.Announcement
	err := authz.RunWithSystemBypassVoid(ctx, "announcement-create", func(ctx context.Context) error {
		return s.RunInTransaction(ctx, func(ctx context.Context) error {
			client := s.entFromContext(ctx)
			targetCount, err := client.APIKey.Query().
				Where(
					apikey.IDIn(apiKeyIDs...),
					apikey.UserIDNotNil(),
					apikey.StatusNEQ(apikey.StatusArchived),
				).
				Count(ctx)
			if err != nil {
				return fmt.Errorf("validate announcement API keys: %w", err)
			}
			if targetCount != len(apiKeyIDs) {
				return fmt.Errorf("one or more selected API keys do not exist or have no user owner")
			}

			created, err = client.Announcement.Create().
				SetContent(content).
				AddAPIKeyIDs(apiKeyIDs...).
				Save(ctx)
			if err != nil {
				return fmt.Errorf("create announcement: %w", err)
			}

			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	return toAnnouncement(created, len(apiKeyIDs)), nil
}

func (s *AnnouncementService) List(ctx context.Context) ([]*Announcement, error) {
	if err := requireAnnouncementScope(ctx, scopes.ScopeReadUsers); err != nil {
		return nil, err
	}

	return authz.RunWithSystemBypass(ctx, "announcement-list", func(ctx context.Context) ([]*Announcement, error) {
		items, err := s.entFromContext(ctx).Announcement.Query().
			WithAPIKeys().
			Order(ent.Desc(announcement.FieldCreatedAt)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list announcements: %w", err)
		}

		return lo.Map(items, func(item *ent.Announcement, _ int) *Announcement {
			return toAnnouncement(item, len(item.Edges.APIKeys))
		}), nil
	})
}

func (s *AnnouncementService) ListForCurrentUser(ctx context.Context) ([]*Announcement, error) {
	currentUser, ok := contexts.GetUser(ctx)
	if !ok || currentUser == nil {
		return nil, fmt.Errorf("user not found in context")
	}

	return authz.RunWithSystemBypass(ctx, "announcement-list-current-user", func(ctx context.Context) ([]*Announcement, error) {
		items, err := s.entFromContext(ctx).Announcement.Query().
			Where(announcement.HasAPIKeysWith(apikey.UserIDEQ(currentUser.ID))).
			Order(ent.Desc(announcement.FieldCreatedAt)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list user announcements: %w", err)
		}

		return lo.Map(items, func(item *ent.Announcement, _ int) *Announcement {
			return toAnnouncement(item, 0)
		}), nil
	})
}

func (s *AnnouncementService) ListRecipientAPIKeys(ctx context.Context) ([]*AnnouncementRecipientAPIKey, error) {
	if err := requireAnnouncementScope(ctx, scopes.ScopeWriteUsers); err != nil {
		return nil, err
	}

	return authz.RunWithSystemBypass(ctx, "announcement-list-recipient-api-keys", func(ctx context.Context) ([]*AnnouncementRecipientAPIKey, error) {
		items, err := s.entFromContext(ctx).APIKey.Query().
			Where(
				apikey.UserIDNotNil(),
				apikey.StatusNEQ(apikey.StatusArchived),
			).
			Order(ent.Asc(apikey.FieldName)).
			All(ctx)
		if err != nil {
			return nil, fmt.Errorf("list announcement recipient API keys: %w", err)
		}

		return lo.Map(items, func(item *ent.APIKey, _ int) *AnnouncementRecipientAPIKey {
			return &AnnouncementRecipientAPIKey{
				ID:   objects.GUID{Type: ent.TypeAPIKey, ID: item.ID},
				Name: item.Name,
			}
		}), nil
	})
}

func (s *AnnouncementService) Delete(ctx context.Context, id int) error {
	if err := requireAnnouncementScope(ctx, scopes.ScopeWriteUsers); err != nil {
		return err
	}

	return authz.RunWithSystemBypassVoid(ctx, "announcement-delete", func(ctx context.Context) error {
		deleted, err := s.entFromContext(ctx).Announcement.Delete().
			Where(announcement.IDEQ(id)).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("delete announcement: %w", err)
		}
		if deleted == 0 {
			return fmt.Errorf("announcement not found")
		}

		return nil
	})
}

func requireAnnouncementScope(ctx context.Context, scope scopes.ScopeSlug) error {
	currentUser, ok := contexts.GetUser(ctx)
	if !ok || currentUser == nil {
		return fmt.Errorf("user not found in context")
	}
	if !scopes.HasSystemScope(currentUser, scope) {
		return fmt.Errorf("permission denied: %s is required", scope)
	}

	return nil
}

func toAnnouncement(item *ent.Announcement, recipientCount int) *Announcement {
	return &Announcement{
		ID:             objects.GUID{Type: ent.TypeAnnouncement, ID: item.ID},
		Content:        item.Content,
		CreatedAt:      item.CreatedAt,
		RecipientCount: recipientCount,
	}
}
