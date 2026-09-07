package repository

import (
	"context"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

// CampaignSenderPool is the set of mailboxes a campaign sends from, resolved
// the one way every caller must agree on: the explicit campaign_senders pool
// and the tag-resolved mailboxes united, and when the campaign selects
// neither, every active mailbox in its organization ("all").
type CampaignSenderPool struct {
	// Accounts is the union, in explicit-pool-first order, without duplicates.
	Accounts []models.Email
	// Explicit is the campaign_senders pool with its rotation metadata.
	Explicit []CampaignSenderAccount
}

// ResolveCampaignSenderPool resolves a campaign's mailboxes. The scheduler and
// the pre-send checks both go through it, so a check can never refuse a pool
// the scheduler would happily send from (issue #340: a campaign on the "all"
// fallback was told it had no sender accounts).
//
// Tenancy is the campaign's organization, never its owner: a user in two
// organizations must not have A's campaign pick up B's mailbox. A campaign
// with no organization resolves to no mailboxes.
func ResolveCampaignSenderPool(ctx context.Context, repo EmailRepository, campaign *models.Campaign) (CampaignSenderPool, *errx.Error) {
	pool := CampaignSenderPool{Accounts: []models.Email{}}
	scope := NewAccountScope(campaign.OrganizationID)
	explicit, err := repo.GetByCampaignSenders(ctx, scope, campaign.ID)
	if err != nil {
		return pool, err
	}
	pool.Explicit = explicit
	seen := map[string]bool{}
	for _, snd := range explicit {
		pool.Accounts = append(pool.Accounts, snd.Account)
		seen[snd.Account.ID.String()] = true
	}
	if len(campaign.EmailTags) > 0 {
		tagged, err := repo.GetByTags(ctx, scope, campaign.EmailTags)
		if err != nil {
			return pool, err
		}
		for _, acct := range tagged {
			if !seen[acct.ID.String()] {
				pool.Accounts = append(pool.Accounts, acct)
				seen[acct.ID.String()] = true
			}
		}
	}
	if len(explicit) == 0 && len(campaign.EmailTags) == 0 {
		all, err := repo.GetAllActiveInScope(ctx, scope)
		if err != nil {
			return pool, err
		}
		pool.Accounts = all
	}
	return pool, nil
}
