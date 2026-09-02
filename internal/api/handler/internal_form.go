package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/app/form"
	"github.com/warmbly/warmbly/internal/config"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/formwire"
	"github.com/warmbly/warmbly/internal/models"
)

// The forms service's slice of the internal API (INTERNAL_API_TOKEN, same
// pattern as the worker DEK proxy and the tracking link resolver): fetch a
// published form, record a funnel event, forward a submission. The service
// owns the public HTML and the per-visitor abuse checks; the pipeline that
// turns answers into contacts stays here.

func formCaptchaSiteKey(f *models.Form) string {
	if !f.CaptchaEnabled || config.CaptchaProvider() == "none" {
		return ""
	}
	return config.TurnstileSiteKey()
}

// InternalGetPublicForm resolves a published form for the forms service.
// 404 for unknown or unpublished ids, so the service can negative-cache. A
// valid ?t= ticket adds the contact's prefill values and echoes the token.
func (h *Handler) InternalGetPublicForm(c *gin.Context) {
	f, xerr := h.FormService.PublicForm(c.Request.Context(), c.Param("publicID"))
	if xerr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	out := formwire.PublicForm{
		PublicID:       f.PublicID,
		Name:           f.Name,
		Fields:         f.Fields,
		Design:         f.Design,
		LogoURL:        f.LogoURL,
		CoverURL:       f.CoverURL,
		BackgroundURL:  f.BackgroundURL,
		AllowedDomains: f.AllowedDomains,
		CaptchaSiteKey: formCaptchaSiteKey(f),
	}
	if token := c.Query("t"); token != "" {
		if link, prefill := h.FormService.ResolveLink(c.Request.Context(), f, token); link != nil {
			out.Prefill = prefill
			out.LinkToken = token
		}
	}
	c.JSON(http.StatusOK, out)
}

// InternalRecordFormEvent stores one funnel event; the forms service already
// deduped views, filtered prefetches and budgeted the source.
func (h *Handler) InternalRecordFormEvent(c *gin.Context) {
	var req formwire.EventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	xerr := h.FormService.RecordEvent(c.Request.Context(), c.Param("publicID"), form.EventInput{
		Type:       req.Type,
		PageIndex:  req.PageIndex,
		PagesTotal: req.PagesTotal,
		VisitorKey: req.VisitorKey,
		SourceURL:  req.SourceURL,
		LinkToken:  req.LinkToken,
		RemoteIP:   req.RemoteIP,
		UserAgent:  req.UserAgent,
	})
	if xerr != nil {
		if xerr.Code == errx.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_event"})
		return
	}
	c.Status(http.StatusNoContent)
}

// InternalSubmitForm runs the submission pipeline for answers the forms
// service collected. The 400 body's message is visitor-facing: the service
// shows it inline on the form.
func (h *Handler) InternalSubmitForm(c *gin.Context) {
	var req formwire.SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, formwire.SubmitError{Error: "invalid_body", Message: "The submission could not be read."})
		return
	}
	meta := form.SubmitMeta{
		RemoteIP:       req.RemoteIP,
		SourceURL:      req.SourceURL,
		CaptchaToken:   req.CaptchaToken,
		HoneypotFilled: req.HoneypotFilled,
		LinkToken:      req.LinkToken,
		VisitorKey:     req.VisitorKey,
	}
	if req.RenderedAt > 0 {
		meta.RenderedAt = time.Unix(req.RenderedAt, 0)
	}
	res, xerr := h.FormService.Submit(c.Request.Context(), c.Param("publicID"), req.Answers, meta)
	if xerr != nil {
		if xerr.Code == errx.NotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		// Message only: the errx prefix ("Bad Request (400):") is for logs,
		// not for a visitor's inline error.
		c.JSON(http.StatusBadRequest, formwire.SubmitError{Error: "form_submit_failed", Message: xerr.Message})
		return
	}
	c.JSON(http.StatusOK, formwire.SubmitResult{Message: res.Message, RedirectURL: res.RedirectURL})
}
