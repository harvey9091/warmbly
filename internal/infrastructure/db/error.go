package db

import (
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
