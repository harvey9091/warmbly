package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/infrastructure/storage"
)

// Internal blob brokering, used by nodes that hold no credential for the
// object store. The control plane signs one operation on one key; the node
// then talks to the store directly, so message bodies and attachments never
// pass through the backend and it pays no bandwidth for a mailbox sync.
//
// Auth is middleware.InternalAuthMiddleware, the same INTERNAL_API_TOKEN every
// other internal route uses.
//
//	POST /api/v1/internal/blobs/presign
//	  body {"op":"get|put|head|delete","key":"...","content_type":"..."}
//	  200  {"url":"...","method":"GET","expires_in":300}
//	  501  the instance's blob backend cannot sign (filesystem)

// blobPresignTTL is how long a signed URL lives. Long enough for a large
// attachment over a slow link, short enough that one captured in a log is
// already dead.
const blobPresignTTL = 5 * time.Minute

type blobPresignRequest struct {
	Op          string `json:"op"`
	Key         string `json:"key"`
	ContentType string `json:"content_type"`
}

type blobPresignResponse struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	ExpiresIn int    `json:"expires_in"`
}

// InternalPresignBlob signs one blob operation for a node.
func (h *Handler) InternalPresignBlob(c *gin.Context) {
	if h.Storage == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "no blob store configured"})
		return
	}
	var req blobPresignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decode body"})
		return
	}
	op := storage.PresignOp(req.Op)
	if !op.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "op must be one of get, put, head, delete"})
		return
	}
	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key required"})
		return
	}

	url, err := h.Storage.PresignedURL(c.Request.Context(), op, req.Key, req.ContentType, blobPresignTTL)
	if err != nil {
		// The filesystem backend cannot sign anything, and a node on another
		// machine could not reach those bytes even if it could: they are on a
		// disk it does not have. Say which it is, because the fix is a
		// different BLOB_PROVIDER rather than a retry.
		if errors.Is(err, storage.ErrUnsupported) {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error": "this instance's blob backend cannot sign URLs; a fleet needs BLOB_PROVIDER=s3",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign blob url"})
		return
	}

	c.JSON(http.StatusOK, blobPresignResponse{
		URL:       url,
		Method:    op.Method(),
		ExpiresIn: int(blobPresignTTL.Seconds()),
	})
}
