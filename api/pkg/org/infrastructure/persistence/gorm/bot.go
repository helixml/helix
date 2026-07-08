package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// botRow has composite PK (id, org_id) so short readable handles
// (`b-root`, `b-engineer`) can repeat across helix tenants. OrgID
// additionally carries a FK to organizations(id) ON DELETE CASCADE —
// added out-of-band in OpenWithDB because GORM tag-driven FK creation
// to a table owned by another package is fragile.
//
// A Bot is the merge of the former Role and Worker: it carries its own
// content + tool list (its capability) and is the live participant in
// the reporting graph. Reporting lines (who reports to whom) are a
// separate many-to-many relation — see reportingLineRow — so a Bot
// carries no parent column.
type botRow struct {
	ID              string   `gorm:"primaryKey;type:text"`
	OrgID           string   `gorm:"primaryKey;type:text;index"`
	Name            string   `gorm:"not null;default:''"`
	Content         string   `gorm:"not null"`
	Tools           []string `gorm:"serializer:json"`
	PreserveContext bool     `gorm:"not null;default:false"`
	// Kind is "" (agent) or "human". HelixUserID / Identity are only
	// populated for human placeholder rows.
	Kind        string            `gorm:"not null;default:'';index"`
	HelixUserID string            `gorm:"not null;default:''"`
	Identity    map[string]string `gorm:"serializer:json"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (botRow) TableName() string { return "org_bots" }

type botMapper struct{}

func (botMapper) ToRow(b orgchart.Bot) (botRow, error) {
	tools := make([]string, 0, len(b.Tools))
	for _, t := range b.Tools {
		tools = append(tools, string(t))
	}
	if len(tools) == 0 {
		tools = nil
	}
	return botRow{
		ID:              string(b.ID),
		OrgID:           b.OrganizationID,
		Name:            b.Name,
		Content:         b.Content,
		Tools:           tools,
		PreserveContext: b.PreserveContext,
		Kind:            b.Kind,
		HelixUserID:     b.HelixUserID,
		Identity:        b.Identity,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}, nil
}

func (botMapper) ToDomain(row botRow) (orgchart.Bot, error) {
	var tools []tool.Name
	if len(row.Tools) > 0 {
		tools = make([]tool.Name, 0, len(row.Tools))
		for _, t := range row.Tools {
			tools = append(tools, tool.Name(t))
		}
	}
	return orgchart.Bot{
		ID:              orgchart.BotID(row.ID),
		OrganizationID:  row.OrgID,
		Name:            row.Name,
		Content:         row.Content,
		Tools:           tools,
		PreserveContext: row.PreserveContext,
		Kind:            row.Kind,
		HelixUserID:     row.HelixUserID,
		Identity:        row.Identity,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

type botsRepo struct {
	*Repository[orgchart.Bot, botRow]
	db *gorm.DB
}

func newBotsRepo(db *gorm.DB) *botsRepo {
	return &botsRepo{
		Repository: NewRepository[orgchart.Bot, botRow](db, botMapper{}, "bot"),
		db:         db,
	}
}

func (r *botsRepo) Get(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Bot, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *botsRepo) List(ctx context.Context, orgID string) ([]orgchart.Bot, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *botsRepo) Update(ctx context.Context, b orgchart.Bot) error {
	row, err := botMapper{}.ToRow(b)
	if err != nil {
		return fmt.Errorf("map bot: %w", err)
	}
	// Pre-marshal JSON columns so the Updates() map carries typed
	// string literals; gorm's serializer:json tag works on full-row
	// Save but not on a map[string]any Updates — pgx can't infer the
	// column type from a bare []string parameter.
	toolsJSON, err := json.Marshal(row.Tools)
	if err != nil {
		return fmt.Errorf("marshal tools: %w", err)
	}
	// Pre-marshal identity for the same reason as tools: the serializer:json
	// tag does not apply on a map[string]any Updates, so pgx can't infer the
	// jsonb column type from a bare map[string]string parameter.
	identityJSON, err := json.Marshal(row.Identity)
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"name":             row.Name,
			"content":          row.Content,
			"tools":            string(toolsJSON),
			"preserve_context": row.PreserveContext,
			"kind":             row.Kind,
			"helix_user_id":    row.HelixUserID,
			"identity":         string(identityJSON),
			"updated_at":       row.UpdatedAt,
		}),
	)
}

// Delete removes the bot row and drops its bot-anchored subscriptions
// in the same transaction. The reporting lines that reference this bot
// (as manager or report) are removed by the ON DELETE CASCADE foreign
// keys on org_reporting_lines (installed in OpenWithDB), so no app code
// clears them — that's the whole point of the association table.
func (r *botsRepo) Delete(ctx context.Context, orgID string, id orgchart.BotID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND bot_id = ?", orgID, string(id)).
			Delete(&subscriptionRow{}).Error; err != nil {
			return fmt.Errorf("delete bot: drop subscriptions: %w", err)
		}
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&botRow{})
		if res.Error != nil {
			return fmt.Errorf("delete bot: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("bot: %w", store.ErrNotFound)
		}
		return nil
	})
}
