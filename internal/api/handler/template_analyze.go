// AI spam analysis for a campaign template. The rules-based score
// (POST /templates/score) says how safe the copy is; this says which word,
// which sentence, and whether it is in the subject or the body, and what to
// write instead. Flow matches every other metered AI endpoint: gate the org,
// reserve credits before the provider call, refund on a provider failure,
// settle real token usage after.
//
// The rules pass runs in the same request and comes back under "rules", so the
// two halves of the editor's panel are scored from one reading of the copy
// rather than from two drafts a keystroke apart.
//
// A third pass, under "judgment", asks TypeSafe how the copy reads to its
// recipient. It costs no credits and needs no LLM, so a deployment with a
// TypeSafe key and no AI provider still answers here, with the rules score and
// the judgment in place of the LLM's findings.
package handler

import (
	"context"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/app/copyjudge"
	"github.com/warmbly/warmbly/internal/app/credits"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/spamcheck"
	"github.com/warmbly/warmbly/internal/pkg/warmlint"
)

// analyzeMaxLen bounds the template a single request may analyze. Well past any
// cold email; a body beyond it is a pasted newsletter, not outreach.
const analyzeMaxLen = 60000

type analyzeTemplateRequest struct {
	Subject   string `json:"subject"`
	BodyHTML  string `json:"body_html"`
	BodyPlain string `json:"body_plain"`
}

// AnalyzeTemplateContent — POST /templates/analyze
func (h *Handler) AnalyzeTemplateContent(c *gin.Context) {
	orgID := middleware.GetOrganizationID(c)
	if orgID == nil {
		errx.JSON(c, errx.New(errx.BadRequest, "no organization selected"))
		return
	}

	var req analyzeTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errx.JSON(c, errx.New(errx.BadRequest, "invalid request body"))
		return
	}
	if len(req.Subject)+len(req.BodyHTML)+len(req.BodyPlain) > analyzeMaxLen {
		errx.JSON(c, errx.New(errx.BadRequest, "this template is too long to analyze"))
		return
	}
	if strings.TrimSpace(req.Subject) == "" && strings.TrimSpace(req.BodyPlain) == "" && strings.TrimSpace(req.BodyHTML) == "" {
		errx.JSON(c, errx.New(errx.BadRequest, "there is nothing written to analyze yet"))
		return
	}

	if h.AIProvider == nil && h.TypeSafe == nil {
		// Identified, because "no provider here" is permanent for this
		// deployment while a provider outage is not, and the editor hides the
		// button only for the first.
		errx.JSON(c, errx.NewWithIdentifier(errx.ServiceUnavailable, "ai_not_configured",
			"AI analysis is not configured on this deployment."))
		return
	}
	allowed, xerr := h.FeatureGateService.CanUseWritingAssistant(c.Request.Context(), *orgID)
	if xerr != nil {
		errx.JSON(c, xerr)
		return
	}
	if !allowed {
		errx.JSON(c, errx.New(errx.Forbidden, "AI spam analysis requires an active plan or trial."))
		return
	}

	rules := warmlint.Score(req.Subject, req.BodyHTML, req.BodyPlain)

	if h.AIProvider == nil {
		// No LLM to quote the copy back, so the judgment is the analysis:
		// the rules score, one sentence on how it reads, nothing charged.
		judgment := h.copyJudgment(c.Request.Context(), *orgID, req)
		if judgment == nil {
			errx.JSON(c, errx.New(errx.ServiceUnavailable, "The copy judge is temporarily unavailable. Nothing was charged."))
			return
		}
		remaining := 0
		if bal, berr := h.CreditService.GetBalance(c.Request.Context(), *orgID); berr == nil {
			remaining = bal
		}
		c.JSON(http.StatusOK, gin.H{
			"score":             rules.Score,
			"verdict":           judgment.Summary(),
			"findings":          []spamcheck.Finding{},
			"rules":             rules,
			"judgment":          judgment,
			"model":             judgment.Model,
			"tokens_used":       judgment.InputTokens,
			"credits_remaining": remaining,
			"credits_charged":   0,
		})
		return
	}

	paid, _ := h.FeatureGateService.IsPaidOrganization(c.Request.Context(), *orgID)
	model := h.AIProvider.ModelForTier(paid)

	idemKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	local := h.AIProvider.IsLocal()
	reqCtx := c.Request.Context()
	{
		meta := models.CreditMeta{Context: models.CreditContext{Detail: "spam analysis"}}
		if actor, aerr := middleware.GetUserUUID(c); aerr == nil {
			meta.ActorID = actor
		}
		reqCtx = models.WithCreditMeta(reqCtx, meta)
	}

	var remaining int
	if local {
		if bal, berr := h.CreditService.GetBalance(reqCtx, *orgID); berr == nil {
			remaining = bal
		}
	} else {
		var cerr error
		remaining, cerr = h.CreditService.Consume(reqCtx, *orgID, credits.CostSpamAnalysis, "spam_analysis", model, 0, idemKey)
		if cerr != nil {
			mapCreditError(c, cerr)
			return
		}
	}

	analysis, aerr := spamcheck.Analyze(c.Request.Context(), h.AIProvider, model, spamcheck.Input{
		Subject:   req.Subject,
		BodyHTML:  req.BodyHTML,
		BodyPlain: req.BodyPlain,
		Rules:     rules,
		Voice:     h.orgVoice(c.Request.Context(), *orgID, ""),
	})
	if aerr != nil {
		refunded := local
		if !local {
			bal, rerr := h.CreditService.Grant(reqCtx, *orgID, credits.CostSpamAnalysis, "spam_analysis_refund")
			if rerr == nil {
				remaining, refunded = bal, true
			} else {
				// Do not tell the customer their credits came back when the
				// refund is what failed. Logged so it can be reconciled from
				// the ledger rather than discovered from a support ticket.
				log.Error().Err(rerr).Str("organization_id", orgID.String()).
					Int("credits", credits.CostSpamAnalysis).
					Msg("spam analysis refund failed after a provider error")
			}
		}
		if !refunded {
			errx.JSON(c, errx.New(errx.ServiceUnavailable,
				"The spam analyzer is temporarily unavailable, and the credits it reserved could not be returned automatically. Contact support and they will be refunded."))
			return
		}
		errx.JSON(c, errx.New(errx.ServiceUnavailable, "The spam analyzer is temporarily unavailable. Your credits were not charged."))
		return
	}

	// Judged only once the paid analysis succeeded, so a request refused for
	// credits or a provider error spends no TypeSafe call.
	judgment := h.copyJudgment(c.Request.Context(), *orgID, req)

	charged := 0
	if !local {
		charged = credits.CostSpamAnalysis
		if extra, serr := h.CreditService.SettleUsage(reqCtx, *orgID, credits.CostSpamAnalysis, analysis.Model, analysis.TokensUsed, "spam_analysis", settleKey(idemKey)); serr == nil && extra > 0 {
			remaining -= extra
			charged += extra
		}
	}

	out := gin.H{
		"score":             analysis.Score,
		"verdict":           analysis.Verdict,
		"findings":          analysis.Findings,
		"suggested_subject": analysis.SuggestedSubject,
		"improvements":      analysis.Improvements,
		"rules":             rules,
		"model":             analysis.Model,
		"tokens_used":       analysis.TokensUsed,
		"credits_remaining": remaining,
		"credits_charged":   charged,
	}
	if judgment != nil {
		out["judgment"] = judgment
	}
	c.JSON(http.StatusOK, out)
}

// copyJudgment asks TypeSafe how the copy reads. Uncached on purpose: a
// re-check is the writer asking again. Nil when unconfigured or unavailable.
func (h *Handler) copyJudgment(ctx context.Context, orgID uuid.UUID, req analyzeTemplateRequest) *copyjudge.Verdict {
	if h.TypeSafe == nil {
		return nil
	}
	jctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	v, err := copyjudge.Judge(jctx, h.TypeSafe, req.Subject, copyjudge.Body(req.BodyPlain, req.BodyHTML))
	if err != nil {
		log.Debug().Err(err).Str("organization_id", orgID.String()).Msg("copy judgment unavailable")
		return nil
	}
	return v
}
