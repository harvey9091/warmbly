package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The wrapped case is the one that matters: a driver error reaches these
// helpers through whatever the call site wrapped it in, and a type assertion
// silently answers false there.
func TestIsForeignKeyViolation(t *testing.T) {
	fk := &pgconn.PgError{Code: "23503", ConstraintName: "email_account_errors_email_account_id_fkey"}

	if !IsForeignKeyViolation(fk) {
		t.Fatal("bare 23503 not detected")
	}
	if !IsForeignKeyViolation(fmt.Errorf("queryrow failed: %w", fk)) {
		t.Fatal("wrapped 23503 not detected")
	}
	if IsForeignKeyViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("unique violation reported as a foreign key violation")
	}
	if IsForeignKeyViolation(errors.New("boom")) || IsForeignKeyViolation(nil) {
		t.Fatal("non-pg error reported as a foreign key violation")
	}
}
