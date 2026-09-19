package db

import "testing"

// The whole point of the clamp is that four processes fit in what the server
// leaves free. A share that rounds up to something comfortable hands out
// capacity the server does not have, which is the exhaustion it exists to
// prevent, so the only floor is the one a pool cannot work without.
func TestProcessShareNeverPromisesMoreThanTheServerHas(t *testing.T) {
	for _, usable := range []int32{4, 8, 16, 20, 40, 76, 97, 400, 4000} {
		share := processShare(usable)
		if total := share * serverShareDivisor; total > usable {
			t.Errorf("usable %d: %d processes at %d each need %d connections", usable, serverShareDivisor, share, total)
		}
	}
}

func TestProcessShareOfProduction(t *testing.T) {
	// 79 max_connections, 3 held for the superuser.
	if got := processShare(76); got != 19 {
		t.Fatalf("expected 19 for the production server, got %d", got)
	}
}

// Below four usable connections the division floors to zero, and a pool of
// zero cannot serve a query. One is the answer, and the caller warns.
func TestProcessShareStaysUsableOnAServerThatIsTooSmall(t *testing.T) {
	for _, usable := range []int32{0, 1, 2, 3} {
		if got := processShare(usable); got != 1 {
			t.Errorf("usable %d: expected a pool of 1, got %d", usable, got)
		}
	}
}

// A server with room keeps the configured ceiling: the clamp in New only
// applies when the share is smaller, and on anything normal it is far larger.
func TestProcessShareLeavesARoomyServerAlone(t *testing.T) {
	if got := processShare(400); got <= defaultMaxConns {
		t.Fatalf("a 400-connection server should not constrain the default of %d, got %d", defaultMaxConns, got)
	}
}
