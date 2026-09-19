package crypt

import "github.com/warmbly/warmbly/internal/errx"

// PasswordError maps a password rejection onto the error the API returns.
// Callers use this rather than testing the boolean so the person setting the
// password is told which rule they hit.
func PasswordError(password string) *errx.Error {
	switch CheckPassword(password) {
	case PasswordTooShort:
		return errx.ErrPassword
	case PasswordTooLong:
		return errx.ErrPasswordTooLong
	case PasswordBreached:
		return errx.ErrPasswordBreached
	}
	return nil
}
