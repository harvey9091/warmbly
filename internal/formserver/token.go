package formserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"time"
)

// The render token is the anti-scraping gate: the JSON API and submit
// endpoints only answer requests carrying a token minted when the page shell
// was served, so a script cannot harvest form definitions or spray submits
// without loading the page first. The key derives from INTERNAL_API_TOKEN,
// so replicas agree without any new configuration.

const renderTokenTTL = 12 * time.Hour

func renderKey(internalToken string) []byte {
	sum := sha256.Sum256([]byte(internalToken + ":forms-render"))
	return sum[:]
}

func renderMAC(key []byte, publicID string, expiry int64) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(publicID + "." + strconv.FormatInt(expiry, 10)))
	return mac.Sum(nil)
}

// mintRenderToken returns base64url(expiryUnix_be8 || HMAC(publicID.expiry)).
func mintRenderToken(key []byte, publicID string, now time.Time) string {
	expiry := now.Add(renderTokenTTL).Unix()
	buf := make([]byte, 8, 8+sha256.Size)
	binary.BigEndian.PutUint64(buf, uint64(expiry))
	buf = append(buf, renderMAC(key, publicID, expiry)...)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func verifyRenderToken(key []byte, publicID, token string, now time.Time) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 8+sha256.Size {
		return false
	}
	expiry := int64(binary.BigEndian.Uint64(raw[:8]))
	if now.Unix() > expiry {
		return false
	}
	return hmac.Equal(raw[8:], renderMAC(key, publicID, expiry))
}
