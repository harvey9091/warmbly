package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/warmbly/warmbly/internal/observability/errs"
)

func CaptureError(err error, query string, params []any, operation string) {
	if err == nil {
		return
	}
	// A cancelled context is the caller going away: a browser navigating off a
	// slow list, a worker shutting down mid-pass, a request that hit its
	// deadline and was already answered. The query did not fail, so reporting
	// it filed the caller's own decision as a database incident and buried the
	// real ones under it. The deadline case is the same event seen from the
	// other side, and the pool reports an in-flight cancellation as a closed
	// connection, which is why that is here too.
	if isCallerGone(err) {
		return
	}
	shape := paramShape(params)
	wrappedErr := fmt.Errorf("%s failed: %w (query: %s, params: %s)", operation, err, query, shape)

	opts := []errs.Option{
		errs.Tag("db.operation", operation),
		errs.Tag("db.query", query),
		errs.Extra("db.params", shape),
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		opts = append(opts,
			errs.Extra("pg.code", pgErr.Code), // e.g., "23505" for unique violation
			errs.Extra("pg.detail", pgErr.Detail),
			errs.Extra("pg.hint", pgErr.Hint),
		)
	}

	errs.CaptureException(wrappedErr, opts...)
}

// isCallerGone reports whether an error only means the caller stopped waiting.
//
// pgx surfaces a cancellation three ways depending on where it landed: as the
// context error itself, as a Postgres 57014 (query_canceled) when the server
// had already started, and as "conn closed" / "failed to deallocate cached
// statement(s)" when the pool tore the connection down with the statement
// still on it. None of the three is a fault in the query.
func isCallerGone(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "57014" {
		return true
	}
	return strings.Contains(err.Error(), "conn closed")
}

// paramShape describes the query's arguments without their values.
//
// The values are the row: email addresses, password hashes, tokens, message
// bodies. The query text next to them already says which column each one is,
// so reporting them would put the most sensitive data in the system into an
// error tracker. How many there were and of what type is enough to tell one
// call site from another, which is all the report is for.
func paramShape(params []any) string {
	if len(params) == 0 {
		return "none"
	}
	kinds := make([]string, 0, len(params))
	for _, p := range params {
		if p == nil {
			kinds = append(kinds, "nil")
			continue
		}
		kinds = append(kinds, fmt.Sprintf("%T", p))
	}
	return strconv.Itoa(len(params)) + " (" + strings.Join(kinds, ", ") + ")"
}
