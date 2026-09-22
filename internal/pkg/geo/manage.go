package geo

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// refreshInterval is how often a long-lived process re-asks whether its
// database has gone stale. Ensure decides that; this only wakes it up.
//
// It matters for a process that outlives maxAge, which a container on a quiet
// week does. Without it, staleness is only ever noticed at boot, and the copy a
// process started with is the copy it dies with.
const refreshInterval = 24 * time.Hour

// Start brings a client's database up to date without holding up boot.
//
// The download used to run inline, so every boot paid for it before the service
// answered its first health check and a mirror that hung delayed the deploy
// rather than just costing city labels. Here the caller gets a usable client
// immediately -- one that reports Unknown -- and the database is swapped in
// underneath it whenever it arrives.
//
// path with no url means the operator manages the file themselves: it is opened
// once and never refreshed. Neither set means geo is off, and this does nothing.
func Start(ctx context.Context, c *Client, path, url string) {
	if c == nil || path == "" {
		return
	}
	go func() {
		// Open whatever is already on disk first. A warm container or a mounted
		// copy should not wait behind a network round trip to start answering.
		if err := c.Use(path); err == nil {
			log.Info().Str("path", path).Msg("geo: database loaded")
		}
		refresh(ctx, c, path, url)
		if url == "" {
			return
		}
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh(ctx, c, path, url)
			}
		}
	}()
}

// refresh runs one Ensure and adopts the result. A failure is reported and
// stepped over: whatever the client is already answering from stays.
func refresh(ctx context.Context, c *Client, path, url string) {
	fetched, err := Ensure(ctx, path, url)
	if err != nil {
		log.Warn().Err(err).Msg("geo: database unavailable; location lookups stay as they are")
		return
	}
	if !fetched {
		return
	}
	if err := c.Use(path); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("geo: downloaded database could not be opened")
		return
	}
	log.Info().Str("path", path).Msg("geo: database updated")
}
