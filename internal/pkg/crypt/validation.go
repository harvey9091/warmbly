package crypt

import (
	"bufio"
	"bytes"
	_ "embed"
	"regexp"
	"strings"
	"sync"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func IsValidUUID(u string) bool {
	return uuidRegex.MatchString(u)
}

func IsValidHexColor(s string) bool {
	return regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`).MatchString(s)
}

// PasswordRejection says why a password was refused, so the caller can tell the
// person which rule they hit rather than restating the length rule at someone
// whose password is long but breached.
type PasswordRejection int

const (
	PasswordOK PasswordRejection = iota
	PasswordTooShort
	PasswordTooLong
	PasswordBreached
)

const (
	passwordMinLength = 8
	passwordMaxLength = 128
)

// breachedList is the NCSC top-100k breached passwords, reduced to the entries
// long enough to pass the length rule. See the README next to it.
//
//go:embed passwords/breached.txt
var breachedList []byte

var (
	breachedOnce sync.Once
	breachedSet  map[string]struct{}
)

// loadBreached builds the lookup set on first use. Parsing ~46k lines costs a
// few milliseconds and only happens on the first password anyone sets.
func loadBreached() {
	breachedSet = make(map[string]struct{}, 48000)
	sc := bufio.NewScanner(bytes.NewReader(breachedList))
	sc.Buffer(make([]byte, 0, 1024), 1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			breachedSet[line] = struct{}{}
		}
	}
}

// IsBreachedPassword reports whether the password appears in the embedded list
// of commonly breached passwords. The comparison is case-insensitive, because
// capitalising the first letter of a breached password does not make it a
// different password to anyone running a cracking list.
func IsBreachedPassword(password string) bool {
	breachedOnce.Do(loadBreached)
	_, found := breachedSet[strings.ToLower(password)]
	return found
}

// CheckPassword applies the password rules: a length band, and a prohibition on
// passwords known to have been breached (NIST SP 800-63B 5.1.1.2, which asks
// for exactly this rather than composition rules).
func CheckPassword(password string) PasswordRejection {
	switch n := len([]rune(password)); {
	case n < passwordMinLength:
		return PasswordTooShort
	case n > passwordMaxLength:
		return PasswordTooLong
	}
	if IsBreachedPassword(password) {
		return PasswordBreached
	}
	return PasswordOK
}

// ValidatePassword is the boolean form of CheckPassword, kept for callers that
// only need to know whether the password is acceptable.
func ValidatePassword(password string) bool {
	return CheckPassword(password) == PasswordOK
}
