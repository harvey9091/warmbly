package orgtransfer

import (
	"testing"

	"github.com/google/uuid"
)

func TestBlobKeyScopeAllows(t *testing.T) {
	org := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	mine := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	theirs := uuid.MustParse("44444444-4444-4444-8444-444444444444")
	s := blobKeyScope{orgID: org, campaigns: map[uuid.UUID]bool{mine: true}}

	ok := []string{
		"email-images/" + org.String() + "/a.png",
		"form-assets/" + org.String() + "/logo.JPG",
		"avatars/users/abc-1-2.png",
		"avatars/organizations/abc-1-2.webp",
		"attachments/" + mine.String() + "/x-report.pdf",
	}
	for _, k := range ok {
		if !s.allows(k) {
			t.Errorf("expected allowed: %q", k)
		}
	}

	bad := []string{
		"",
		"form-assets/" + org.String() + "/evil.svg",  // script-capable image
		"form-assets/" + org.String() + "/evil.html", // stored HTML
		"form-assets/" + org.String() + "/evil.png.html",
		"attachments/" + theirs.String() + "/x.pdf", // another workspace's campaign
		"attachments/not-a-uuid/x.pdf",
		"users/" + org.String() + "/emails/x/y.emsg", // mailbox bodies never travel
		"emails/anything",
		"../../etc/passwd",
		"form-assets/" + org.String() + "/../../avatars/users/x.png",
		"/form-assets/" + org.String() + "/a.png",
		"avatars/other/x.png",
		"form-assets/" + org.String() + "/a.png/extra",
	}
	for _, k := range bad {
		if s.allows(k) {
			t.Errorf("expected refused: %q", k)
		}
	}
}

// An archive made on another instance names the SOURCE workspace in the keys of
// its public objects, because that is the path the bytes lived at there. Those
// keys are rewritten to the importing workspace rather than refused: refusing
// would make every cross-instance import silently lose its images, and
// honouring the source id would let a crafted archive write into another
// workspace's prefix.
func TestBlobKeyScopeRewritesTheWorkspaceSegment(t *testing.T) {
	dest := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	source := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	campaign := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	s := blobKeyScope{orgID: dest, campaigns: map[uuid.UUID]bool{campaign: true}}

	cases := []struct{ in, want string }{
		{"email-images/" + source.String() + "/a.png", "email-images/" + dest.String() + "/a.png"},
		{"form-assets/" + source.String() + "/logo.png", "form-assets/" + dest.String() + "/logo.png"},
		// Already ours: unchanged, which is the same-instance restore case.
		{"email-images/" + dest.String() + "/a.png", "email-images/" + dest.String() + "/a.png"},
		// Not workspace-scoped, so nothing to rewrite.
		{"attachments/" + campaign.String() + "/x.pdf", "attachments/" + campaign.String() + "/x.pdf"},
		{"avatars/users/a-1-2.png", "avatars/users/a-1-2.png"},
	}
	for _, tc := range cases {
		got, ok := s.plan(tc.in)
		if !ok {
			t.Errorf("plan(%q) refused a legitimate key", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("plan(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{
		"form-assets/" + source.String() + "/evil.svg",
		"users/" + source.String() + "/emails/x/y.emsg",
		"attachments/44444444-4444-4444-8444-444444444444/x.pdf",
		"form-assets/not-a-uuid/a.png",
	} {
		if _, ok := s.plan(bad); ok {
			t.Errorf("plan(%q) should have refused", bad)
		}
	}
}

// The registry gate is what keeps an archive's table and column strings out of
// a query.
func TestPublicURLColumnMatchesTheRegistryOnly(t *testing.T) {
	if !publicURLColumn("forms", "logo_url") {
		t.Error("forms.logo_url is a declared public-URL blob column")
	}
	for _, tc := range [][2]string{
		{"forms", "name"},                  // real table, not a blob column
		{"campaign_attachments", "s3_key"}, // a blob column, but a key not a URL
		{"users", "email"},                 // not a blob-carrying table
		{`forms"; DROP TABLE forms; --`, "logo_url"},
		{"forms", `logo_url"; DROP TABLE forms; --`},
	} {
		if publicURLColumn(tc[0], tc[1]) {
			t.Errorf("publicURLColumn(%q, %q) should be false", tc[0], tc[1])
		}
	}
}
