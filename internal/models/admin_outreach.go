package models

import (
	"time"

	"github.com/google/uuid"
)

// AdminOutreachStatus mirrors the postgres enum from migration 000047.
type AdminOutreachStatus string

const (
	AdminOutreachStatusQueued AdminOutreachStatus = "queued"
	AdminOutreachStatusSent   AdminOutreachStatus = "sent"
	AdminOutreachStatusFailed AdminOutreachStatus = "failed"
)

// AdminOutreachMessage is one row of the admin-outreach audit log.
// to_user_id / to_org_id are nil when the admin types a raw email
// address; to_email is always populated.
type AdminOutreachMessage struct {
	ID        uuid.UUID           `json:"id"`
	SentBy    uuid.UUID           `json:"sent_by"`
	ToUserID  *uuid.UUID          `json:"to_user_id,omitempty"`
	ToOrgID   *uuid.UUID          `json:"to_org_id,omitempty"`
	ToEmail   string              `json:"to_email"`
	ReplyTo   *string             `json:"reply_to,omitempty"`
	Subject   string              `json:"subject"`
	Body      string              `json:"body"`
	Status    AdminOutreachStatus `json:"status"`
	Error     *string             `json:"error,omitempty"`
	SentAt    *time.Time          `json:"sent_at,omitempty"`
	CreatedAt time.Time           `json:"created_at"`

	// Joined data.
	SentByUser    *AdminUserSummary `json:"sent_by_user,omitempty"`
	ToUserSummary *AdminUserSummary `json:"to_user,omitempty"`
}

// SendAdminOutreachRequest is what the composer POSTs. Exactly one of
// to_user_id / to_org_id / to_email must be set; the service resolves
// the canonical recipient address from there.
type SendAdminOutreachRequest struct {
	ToUserID *uuid.UUID `json:"to_user_id,omitempty"`
	ToOrgID  *uuid.UUID `json:"to_org_id,omitempty"`
	ToEmail  string     `json:"to_email,omitempty"`
	ReplyTo  string     `json:"reply_to,omitempty"`
	Subject  string     `json:"subject" binding:"required,min=1,max=998"`
	Body     string     `json:"body" binding:"required,min=1"`
}

// AdminOutreachSearch are the query params for the admin outreach log listing.
// Mirrors AdminOrgSearch form-tag conventions; date facets are *time.Time with
// time_format/time_utc so gin's ShouldBindQuery parses YYYY-MM-DD.
type AdminOutreachSearch struct {
	Query         string `form:"q"`
	Status        string `form:"status"`         // queued, sent, failed
	RecipientType string `form:"recipient_type"` // user, org, email
	SentByQuery   string `form:"sent_by_q"`

	HasReplyTo bool `form:"has_reply_to"`
	HasError   bool `form:"has_error"`
	HasUser    bool `form:"has_user"`
	HasOrg     bool `form:"has_org"`

	// Date ranges (YYYY-MM-DD, UTC)
	CreatedWithin int        `form:"created_within"` // days; 0 = any
	CreatedAfter  *time.Time `form:"created_after" time_format:"2006-01-02" time_utc:"true"`
	CreatedBefore *time.Time `form:"created_before" time_format:"2006-01-02" time_utc:"true"`
	SentAtAfter   *time.Time `form:"sent_at_after" time_format:"2006-01-02" time_utc:"true"`
	SentAtBefore  *time.Time `form:"sent_at_before" time_format:"2006-01-02" time_utc:"true"`

	Offset   int    `form:"-"` // decoded from ?cursor by the handler
	Limit    int    `form:"limit"`
	SortBy   string `form:"sort_by"` // created_at, sent_at, status, to_email, subject
	SortDesc bool   `form:"sort_desc"`
}

// AdminOutreachResult is the paginated response for the admin outreach log.
type AdminOutreachResult struct {
	Data       []AdminOutreachMessage `json:"data"`
	Pagination Pagination             `json:"pagination"`
}
