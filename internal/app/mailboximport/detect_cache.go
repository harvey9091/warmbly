package mailboximport

import (
	"context"
	"time"

	"github.com/warmbly/warmbly/internal/infrastructure/cache"
	"github.com/warmbly/warmbly/internal/pkg/mailhost"
)

// detectionTTL is how long a domain's host stays known; a preview re-runs on every mapping change.
const detectionTTL = 6 * time.Hour

// RedisDetectionCache keeps domain detections in Redis, shared by every backend.
type RedisDetectionCache struct {
	c *cache.Cache
}

func NewRedisDetectionCache(c *cache.Cache) *RedisDetectionCache {
	return &RedisDetectionCache{c: c}
}

func (r *RedisDetectionCache) Get(ctx context.Context, domain string) (mailhost.Detection, bool) {
	var d mailhost.Detection
	if r == nil || r.c == nil {
		return d, false
	}
	if err := r.c.GetJSON(ctx, "mailhost:"+domain, &d); err != nil || d.Domain == "" {
		return mailhost.Detection{}, false
	}
	return d, true
}

func (r *RedisDetectionCache) Set(ctx context.Context, domain string, d mailhost.Detection) {
	if r == nil || r.c == nil {
		return
	}
	_ = r.c.SetJSON(ctx, "mailhost:"+domain, d, detectionTTL)
}
