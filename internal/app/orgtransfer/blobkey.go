package orgtransfer

import (
	"context"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/models"
)

// The object keys an archive may write, and nothing else.
//
// A key in the manifest is a string from a file the customer uploaded, so it is
// attacker-controlled in exactly the way a filename is, and it used to be
// handed to the blob store verbatim. Two things followed from that. A crafted
// archive could write over another workspace's objects by naming their keys,
// because a key carries its own scope and nothing checked it against the
// workspace doing the import. And a key under a public prefix is served from
// `/public` with a Content-Type derived from its extension, so `evil.svg` or
// `evil.html` became stored script on this instance's own origin, reachable
// without signing in. Every upload handler forces the extension for that exact
// reason; the import path was the way around them.
//
// So a key is accepted only when it matches a shape this product actually
// mints, for this destination workspace.
const maxBlobKeyLength = 512

// publicBlobExtensions is the set of extensions the upload handlers can produce
// under a public prefix. It is deliberately the same closed image set: anything
// a browser will execute is absent, and so is SVG, which is an image that can
// carry script.
var publicBlobExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

// avatarKinds mirrors the two avatar owners the product writes.
var avatarKinds = map[string]bool{"users": true, "organizations": true}

// blobKeyScope carries what restoreBlobs needs to judge a key: the workspace
// being written to, and the campaigns that workspace owns after the rows have
// been imported.
type blobKeyScope struct {
	orgID     uuid.UUID
	campaigns map[uuid.UUID]bool
}

// loadBlobKeyScope reads the destination's campaign ids inside the import
// transaction. It runs after the tables are written, so a campaign that arrived
// in this very archive counts, and one belonging to another workspace does not.
func loadBlobKeyScope(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) (blobKeyScope, error) {
	scope := blobKeyScope{orgID: orgID, campaigns: map[uuid.UUID]bool{}}
	rows, err := tx.Query(ctx, `SELECT id FROM campaigns WHERE organization_id = $1`, orgID)
	if err != nil {
		return scope, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return scope, err
		}
		scope.campaigns[id] = true
	}
	return scope, rows.Err()
}

// plan decides whether the archive may write this key, and under what key it
// should actually be written.
//
// The two org-scoped public prefixes carry the SOURCE workspace's id, because
// that is the path the bytes lived at on the instance that exported them. On a
// cross-instance move the destination workspace has a different id, so the
// segment is rewritten rather than checked: validating it would refuse every
// legitimate archive, and honouring it would let a crafted one write into
// another workspace's prefix. Rewriting does both jobs at once.
//
// The caller has to store the returned key, not the one in the manifest, and
// repoint the row that references it.
func (s blobKeyScope) plan(key string) (string, bool) {
	if !s.allows(key) {
		return "", false
	}
	parts := strings.Split(key, "/")
	switch parts[0] + "/" {
	case models.EmailImageKeyPrefix, "form-assets/":
		parts[1] = s.orgID.String()
		return strings.Join(parts, "/"), true
	}
	return key, true
}

// allows reports whether the archive may write this key on this instance.
func (s blobKeyScope) allows(key string) bool {
	if key == "" || len(key) > maxBlobKeyLength {
		return false
	}
	// Reject before normalising: "a/../b" cleaning down to a valid-looking key
	// is not the key the archive named, and a key that needs cleaning is not
	// one this product mints.
	if strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00") {
		return false
	}
	parts := strings.Split(key, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}

	switch parts[0] + "/" {
	case models.EmailImageKeyPrefix, "form-assets/":
		// email-images/<orgID>/<name>.<ext>, form-assets/<orgID>/<name>.<ext>.
		// Both are served from /public, so the extension has to be an image.
		// The workspace segment is not compared: it names the SOURCE workspace
		// and plan rewrites it to this one, which is what keeps a crafted
		// archive out of another workspace's prefix. It still has to be a uuid,
		// so the shape cannot be used to smuggle anything.
		if len(parts) != 3 || !publicImageName(parts[2]) {
			return false
		}
		_, err := uuid.Parse(parts[1])
		return err == nil

	case "avatars/":
		// avatars/{users,organizations}/<name>.<ext>. Also public.
		return len(parts) == 3 && avatarKinds[parts[1]] && publicImageName(parts[2])

	case "attachments/":
		// attachments/<campaignID>/<name>. Private, so the extension is the
		// sender's business, but the campaign has to be one this workspace owns.
		if len(parts) != 3 {
			return false
		}
		campaignID, err := uuid.Parse(parts[1])
		if err != nil {
			return false
		}
		return s.campaigns[campaignID]
	}

	return false
}

// publicImageName accepts a single path element that ends in an image
// extension. The extension decides the Content-Type the public route serves, so
// this is the check that keeps script off this origin.
func publicImageName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	return publicBlobExtensions[strings.ToLower(path.Ext(name))]
}

// publicURLColumn reports whether (table, column) is a column the registry
// declares as holding a browser-loadable blob URL.
//
// Both values come out of the archive manifest, which is a file the customer
// uploaded, so neither may reach a query until it has been matched against the
// compiled registry. Matching here means the identifiers used later are the
// registry's own constants, not the archive's strings.
func publicURLColumn(table, column string) bool {
	for i := range Tables {
		t := &Tables[i]
		if t.Name != table {
			continue
		}
		for _, b := range t.Blobs {
			if b.Column == column && b.Kind == BlobKindPublicURL {
				return true
			}
		}
		return false
	}
	return false
}
