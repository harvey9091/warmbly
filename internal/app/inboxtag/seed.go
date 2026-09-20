package inboxtag

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/config"
)

// Seeder creates the label taxonomy for a workspace, so the inbox scope rail
// and the premade views work from the moment the workspace exists rather than
// appearing one label at a time as mail is classified.
type Seeder interface {
	EnsureAll(ctx context.Context, orgID uuid.UUID, slugs []string) error
}

// SeedLabels returns the hook a workspace-creation path calls. A workspace
// is never shown a label nothing on its instance can produce.
func SeedLabels(c Seeder) func(ctx context.Context, orgID uuid.UUID) {
	return func(ctx context.Context, orgID uuid.UUID) {
		if c == nil || orgID == uuid.Nil {
			return
		}
		if err := c.EnsureAll(ctx, orgID, SeedSet()); err != nil {
			log.Warn().Err(err).Str("org_id", orgID.String()).Msg("inbox tagging: could not seed labels for a new workspace")
		}
	}
}

// SeedSet is the taxonomy an instance can actually produce.
func SeedSet() []string {
	if config.InboxTaggingEnabled() {
		return AllLabels()
	}
	return FollowUpLabels
}
