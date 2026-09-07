package imap

import (
	"net"
	"sync"
	"time"
)

// idleConn is the transport under one session. go-imap puts a deadline on
// the bytes of a response once its first byte has arrived, but waits for that
// first byte with no deadline at all, so a peer that vanished without a FIN
// (a NAT or firewall dropping the mapping) parks the command forever and the
// mailbox with it. While a command is in flight this keeps a deadline on the
// wait too; between commands it is cleared, because go-imap's reader blocks in
// Read the whole time and would otherwise time an idle session out.
type idleConn struct {
	net.Conn
	timeout time.Duration

	// mu guards inflight together with the deadline calls it drives. A
	// counter alone races: the sync loop and a warmup action run commands on
	// one session concurrently, so the last release can clear the deadline
	// just after another command armed it, leaving that command waiting on a
	// silent peer forever, which is the failure this type exists to prevent.
	mu       sync.Mutex
	inflight int
}

// arm starts the clock for one command. The returned func stops it; every
// command path calls it on exit.
func (c *idleConn) arm() func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight++
	if c.inflight == 1 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.timeout))
	}
	return c.release
}

func (c *idleConn) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight--
	if c.inflight == 0 {
		_ = c.Conn.SetReadDeadline(time.Time{})
	}
}

// SetReadDeadline is where go-imap manages its own per-response deadline: it
// sets one while decoding a response and clears it in between. The cleared
// stretch is the gap: that is where the reader waits for the first byte of
// the next response, with nothing to fail it if the peer went away without a
// FIN. While a command is in flight the clear becomes our timeout instead.
//
// A deadline go-imap set itself is left alone, never shortened: it already
// allows five minutes for a large literal, and cutting that would fail a slow
// body fetch that is making progress.
func (c *idleConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.IsZero() && c.inflight > 0 {
		t = time.Now().Add(c.timeout)
	}
	return c.Conn.SetReadDeadline(t)
}

// Write bounds a send too: a large APPEND to a dead peer blocks once the
// socket buffer is full, and nothing else would ever fail it.
func (c *idleConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	armed := c.inflight > 0
	if armed {
		_ = c.Conn.SetWriteDeadline(time.Now().Add(c.timeout))
	}
	c.mu.Unlock()

	n, err := c.Conn.Write(p)

	if armed {
		c.mu.Lock()
		// Only clear it if nothing else is mid-command: another writer may
		// have armed its own deadline while this write was in flight.
		if c.inflight == 0 {
			_ = c.Conn.SetWriteDeadline(time.Time{})
		}
		c.mu.Unlock()
	}
	return n, err
}
