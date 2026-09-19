package db

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type DB struct {
	*pgxpool.Pool
}

const (
	// MaxConns sizing rationale: 4 was catastrophically low — a single
	// leaked tx in a request handler is enough to deadlock the entire
	// backend (e.g. /auth/refresh blocks waiting for a connection, the
	// frontend treats the resulting timeout as session expiry, user is
	// kicked at the 10-minute refresh boundary). 25 leaves headroom even
	// under bursty admin pages while staying well under postgres'
	// default max_connections=100.
	defaultMaxConns = int32(25)
	defaultMinConns = int32(2)
	// A pooled connection is never given back while it may still be reused,
	// so the pool settles at its high-water mark and holds it. Half an hour
	// of that put 40 idle connections on a 79-connection server for work
	// that had finished minutes earlier. A minute is long enough to reuse a
	// connection across one burst and short enough that the burst's peak is
	// not still charged to us when the next service needs a slot.
	defaultMaxConnLifetime   = time.Hour
	defaultMaxConnIdleTime   = time.Minute
	defaultHealthCheckPeriod = time.Minute
	defaultConnectTimeout    = time.Second * 5

	// How many pooled processes one database is assumed to carry: backend and
	// consumer, each counted twice because a rolling deploy runs the outgoing
	// and incoming container at once. Used only to lower a pool that the
	// server cannot honour, never to raise one.
	serverShareDivisor = int32(4)
	// The floor the clamp will not go under. Four connections deadlocked the
	// backend once already (see above), so a server too small for the fleet
	// gets a loud warning and a working process, not a strangled one.
	minClampedMaxConns = int32(10)

	// Postgres idle-in-transaction safety net. If a code path forgets
	// `defer tx.Rollback(ctx)`, the server will abort the leaked tx
	// after this many milliseconds (5 min) instead of holding the
	// connection forever. Belt-and-suspenders against the bug class
	// that caused the 10-minute logout.
	idleInTxnTimeoutMs = "300000"
	// Statement-level safety net for query runaway. 60s should be more
	// than enough for any user-facing query; admin reports that need
	// longer can override with SET LOCAL statement_timeout.
	statementTimeoutMs = "60000"
)

func New(ctx context.Context, endpoint string) (*DB, error) {
	dbConfig, err := pgxpool.ParseConfig(endpoint)
	if err != nil {
		return nil, err
	}

	// The pool ceiling is per process, and the database's is for the whole
	// fleet: every backend replica, every consumer and every warmblyctl run
	// takes its own share. Exhausting max_connections shows up as "remaining
	// connection slots are reserved", which no amount of retrying fixes, so
	// the size has to be tunable without a rebuild. A DSN that names
	// pool_max_conns keeps it; DB_MAX_CONNS overrides both.
	if dbConfig.MaxConns <= 0 {
		dbConfig.MaxConns = defaultMaxConns
	}
	if n := envInt32("DB_MAX_CONNS"); n > 0 {
		dbConfig.MaxConns = n
	}
	dbConfig.MinConns = defaultMinConns
	if n := envInt32("DB_MIN_CONNS"); n >= 0 && os.Getenv("DB_MIN_CONNS") != "" {
		dbConfig.MinConns = n
	}
	if dbConfig.MinConns > dbConfig.MaxConns {
		dbConfig.MinConns = dbConfig.MaxConns
	}

	// A constant cannot know the server's ceiling, and the same 25 that is
	// generous on a large instance is fatal on a small one: backend plus
	// consumer, doubled by a deploy overlap, asked for more than the server
	// had and every job in that second failed with "remaining connection
	// slots are reserved". Ask the server instead. The probe costs one
	// connection at boot and can only lower the pool.
	if share := serverConnectionShare(ctx, dbConfig.ConnConfig); share > 0 && share < dbConfig.MaxConns {
		log.Warn().
			Int32("configured_max_conns", dbConfig.MaxConns).
			Int32("clamped_max_conns", share).
			Msg("db: pool clamped to this process's share of the server's max_connections; raise the server's limit or set DB_MAX_CONNS to size it yourself")
		dbConfig.MaxConns = share
		if dbConfig.MinConns > dbConfig.MaxConns {
			dbConfig.MinConns = dbConfig.MaxConns
		}
	}
	dbConfig.MaxConnLifetime = defaultMaxConnLifetime
	dbConfig.MaxConnIdleTime = defaultMaxConnIdleTime
	dbConfig.HealthCheckPeriod = defaultHealthCheckPeriod
	dbConfig.ConnConfig.ConnectTimeout = defaultConnectTimeout

	if dbConfig.ConnConfig.RuntimeParams == nil {
		dbConfig.ConnConfig.RuntimeParams = map[string]string{}
	}
	dbConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = idleInTxnTimeoutMs
	dbConfig.ConnConfig.RuntimeParams["statement_timeout"] = statementTimeoutMs

	conn, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		return nil, err
	}

	return &DB{
		Pool: conn,
	}, nil
}

// serverConnectionShare reports the largest pool this process may take from
// the server, or 0 when the server cannot be asked. Boot must not depend on
// the answer: a probe that fails leaves the configured size in place.
func serverConnectionShare(ctx context.Context, cfg *pgx.ConnConfig) int32 {
	probeCtx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	defer cancel()

	conn, err := pgx.ConnectConfig(probeCtx, cfg)
	if err != nil {
		log.Debug().Err(err).Msg("db: could not probe max_connections; keeping the configured pool size")
		return 0
	}
	defer func() { _ = conn.Close(context.WithoutCancel(probeCtx)) }()

	// Both reservations are subtracted because neither is ours to spend: the
	// superuser slots are what the error names, and RDS keeps its own on top.
	// current_setting's second argument suppresses the error for the RDS-only
	// name so the same query works on stock Postgres.
	const q = `SELECT current_setting('max_connections')::int
		- current_setting('superuser_reserved_connections')::int
		- COALESCE(NULLIF(current_setting('rds.rds_superuser_reserved_connections', true), '')::int, 0)`
	var usable int32
	if err := conn.QueryRow(probeCtx, q).Scan(&usable); err != nil {
		log.Debug().Err(err).Msg("db: could not read max_connections; keeping the configured pool size")
		return 0
	}
	if usable <= 0 {
		return 0
	}

	share := usable / serverShareDivisor
	if share < minClampedMaxConns {
		log.Warn().
			Int32("usable_server_connections", usable).
			Msg("db: the server has too few connections for the fleet; every service will contend for slots until its max_connections is raised")
		share = minClampedMaxConns
	}
	return share
}

// envInt32 reads a positive pool bound from the environment. Anything unset or
// unparseable returns -1 so the caller keeps its own default rather than
// sizing the pool from a typo.
func envInt32(name string) int32 {
	raw := os.Getenv(name)
	if raw == "" {
		return -1
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < 0 {
		return -1
	}
	return int32(n)
}
