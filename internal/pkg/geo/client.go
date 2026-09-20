package geo

import (
	"sync"

	"github.com/oschwald/geoip2-golang/v2"
)

// Client answers location lookups from a MaxMind database that may not be
// there yet.
//
// The reader is swappable because the database no longer has to be present at
// boot: a client is created immediately, answers "Unknown" while the download
// runs, and starts answering properly the moment Use hands it a reader. That is
// what keeps a slow or dead mirror from delaying a deploy's health check.
//
// Guarded by a mutex rather than an atomic pointer, deliberately. The reader
// memory-maps its file, so closing one while a lookup is inside it is a
// segfault; holding the read lock for the whole lookup means Use cannot close
// the old reader until every in-flight lookup has left it. A read lock on an
// uncontended RWMutex costs nothing next to the lookup it guards.
type Client struct {
	mu sync.RWMutex
	r  *geoip2.Reader
}

// New opens the database at location, or returns a client with none when
// location is empty. A client with no database is fully usable; it reports
// every address as Unknown.
func New(location string) (*Client, error) {
	c := &Client{}
	if location == "" {
		return c, nil
	}
	db, err := geoip2.Open(location)
	if err != nil {
		return nil, err
	}
	c.r = db
	return c, nil
}

// Use points the client at the database now at location, closing whatever it
// was using. A client that had none starts answering; one that had an older
// copy picks up the newer one without a restart.
func (c *Client) Use(location string) error {
	db, err := geoip2.Open(location)
	if err != nil {
		return err
	}
	c.mu.Lock()
	old := c.r
	c.r = db
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	old := c.r
	c.r = nil
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

// reader hands back the current database, or nil when there is none, together
// with the read lock's release. The caller must call the returned func.
func (c *Client) reader() (*geoip2.Reader, func()) {
	if c == nil {
		return nil, func() {}
	}
	c.mu.RLock()
	if c.r == nil {
		c.mu.RUnlock()
		return nil, func() {}
	}
	return c.r, c.mu.RUnlock
}
