package user

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func getUserKey(id uuid.UUID) string {
	return "user:" + id.String()
}

// SaveUser refreshes the cached copy of a row that lives in Postgres, so a
// cache that will not take it is not a failed write: the authoritative row is
// already there and the read path falls back to it.
//
// A set that fails is followed by a delete, because the danger is not an
// absent copy but a stale one: the copy this call was replacing would
// otherwise be served until its TTL. Both failing means the cache is
// unreachable, which is also the state in which nothing can read the stale
// copy either.
func (s *userService) SaveUser(ctx context.Context, user *models.User) *errx.Error {
	raw, err := json.Marshal(user)
	if err != nil {
		// Not a cache fault: this row cannot be represented at all, and the
		// caller is owed the failure.
		errs.CaptureException(err)
		return errx.InternalError()
	}

	key := getUserKey(user.ID)
	if err := s.cache.SetEx(ctx, key, raw, UserTTL).Err(); err != nil {
		errs.CaptureException(err, errs.Tag("cache.key", "user"), errs.Tag("cache.op", "set"))
		s.cache.Del(ctx, key)
	}

	return nil
}

// getUser answers from the cached copy. A cache that cannot answer is a miss,
// not a failed request: the row is in Postgres and GetUser reads it from there.
//
// Treating an unreachable cache as a fault turned one cache outage into every
// signed-in request answering 500, because the user behind a request is read
// on nearly all of them.
func (s *userService) getUser(ctx context.Context, userID uuid.UUID) (*models.User, *errx.Error) {
	data, err := s.cache.Get(ctx, getUserKey(userID)).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			errs.CaptureException(err, errs.Tag("cache.key", "user"), errs.Tag("cache.op", "get"))
		}
		return nil, nil
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		// Unreadable cached bytes are a miss for the same reason, and the
		// copy is dropped so the next read does not repeat the work.
		errs.CaptureException(err, errs.Tag("cache.key", "user"))
		s.cache.Del(ctx, getUserKey(userID))
		return nil, nil
	}

	return &user, nil
}
