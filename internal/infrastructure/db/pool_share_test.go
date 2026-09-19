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

// pgxpool's ParseConfig fills MaxConns with max(4, NumCPU) before we see it,
// so the `<= 0` guard this replaced never fired and defaultMaxConns was dead
// code. Production ran at 48 per process, the host's core count, while the
// constant and the docs both said 25.
func TestPoolCeilingIgnoresTheHostsCoreCount(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "")

	const dsn = "postgres://u:p@host:5432/db?sslmode=require"
	if got := poolCeiling(dsn, 48); got != defaultMaxConns {
		t.Errorf("a 48-core host gave a ceiling of %d, want the default %d", got, defaultMaxConns)
	}
	if got := poolCeiling(dsn, 4); got != defaultMaxConns {
		t.Errorf("a small host gave a ceiling of %d, want the default %d", got, defaultMaxConns)
	}
}

// A DSN that names the setting means someone chose it, so it is kept.
func TestPoolCeilingKeepsAnExplicitDSNSetting(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "")

	const dsn = "postgres://u:p@host:5432/db?pool_max_conns=40"
	if got := poolCeiling(dsn, 40); got != 40 {
		t.Errorf("ceiling = %d, want the 40 the DSN asked for", got)
	}
}

func TestPoolCeilingLetsTheEnvironmentWin(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "12")

	if got := poolCeiling("postgres://u:p@host:5432/db?pool_max_conns=40", 40); got != 12 {
		t.Errorf("ceiling = %d, want the environment's 12", got)
	}
	if got := poolCeiling("postgres://u:p@host:5432/db", 48); got != 12 {
		t.Errorf("ceiling = %d, want the environment's 12", got)
	}
}
