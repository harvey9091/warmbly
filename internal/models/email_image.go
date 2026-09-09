package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailImage is one image in a workspace's library for email bodies. The bytes
// live in object storage under a public key (URL), because the recipient's mail
// client fetches them with no session of ours; the row is what the composer
// lists and what the storage quota counts.
type EmailImage struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	UserID         *uuid.UUID `json:"user_id,omitempty"`
	Filename       string     `json:"filename"`
	MimeType       string     `json:"mime_type"`
	Size           int64      `json:"size"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	StorageKey     string     `json:"-"`
	URL            string     `json:"url"`
	CreatedAt      time.Time  `json:"created_at"`
}

// EmailImageKeyPrefix is the public object-key prefix email-body images live
// under. Public because a mail client loads them unauthenticated; the prefix is
// what /public/*key and the org-archive blob collector both match on.
const EmailImageKeyPrefix = "email-images/"

// EmailImageObjectKey is where an email-body image's bytes live.
//
// The uploader's filename is deliberately NOT part of it. This key becomes a
// URL inside an email, so a name like "acme-q3-pricing-internal.png" would be
// read by every recipient; and a name is free text, so one holding ".." would
// produce a key the public route refuses to serve. The name is kept on the row
// instead, where the library lists it and the alt text defaults to it.
func EmailImageObjectKey(orgID uuid.UUID, ext string) string {
	return EmailImageKeyPrefix + orgID.String() + "/" + uuid.NewString() + ext
}
