package imap

import (
	"net"
	"sync"
	"testing"
	"time"
)

// Commands overlap on one session: the sync loop fetches while a warmup
// action stores a flag. If the last release could clear the deadline just
// after another command armed it, that command would wait on a silent peer
// forever, which is the whole failure this type exists to prevent. Run under
// -race, this also proves the deadline state is not touched concurrently.
func TestIdleConnStaysArmedWhileAnyCommandIsInFlight(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		select {}
	}()

	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := &idleConn{Conn: raw, timeout: 200 * time.Millisecond}

	// One long-running command, with short ones starting and finishing
	// underneath it the whole time.
	outer := conn.arm()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := conn.arm()
			_ = conn.SetReadDeadline(time.Time{})
			release()
		}()
	}
	wg.Wait()

	// The outer command is still in flight, so its wait must still be bounded.
	start := time.Now()
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("a read against a silent peer succeeded")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("the read waited %v: a finishing command cleared the deadline out from under one still in flight", waited)
	}
	outer()
}
