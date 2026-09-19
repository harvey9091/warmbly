package crypt

import "testing"

func TestCheckPassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		want     PasswordRejection
	}{
		{"too short", "Sh0rt!", PasswordTooShort},
		{"exactly at the floor", "Xj7!qm2Z", PasswordOK},
		{"a long passphrase", "correct horse battery staple", PasswordOK},
		{"breached", "password123", PasswordBreached},
		{"breached, recased", "PassWord123", PasswordBreached},
		{"breached, all caps", "QWERTY123", PasswordBreached},
		{"over the ceiling", string(make([]byte, 129)), PasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckPassword(tc.password); got != tc.want {
				t.Fatalf("CheckPassword(%q) = %v, want %v", tc.password, got, tc.want)
			}
		})
	}
}

// The denylist is only worth carrying if it is actually populated: an empty or
// truncated data file would make every password acceptable and nothing else
// would notice.
func TestBreachedListIsLoaded(t *testing.T) {
	breachedOnce.Do(loadBreached)
	if len(breachedSet) < 40000 {
		t.Fatalf("breached list holds %d entries, expected the full NCSC set", len(breachedSet))
	}
	for _, p := range []string{"password123", "qwerty123", "letmein1"} {
		if !IsBreachedPassword(p) {
			t.Errorf("%q should be in the breached list", p)
		}
	}
	// A password nobody has leaked must not be refused, or the control is just
	// an outage.
	if IsBreachedPassword("Xj7!qm2Zp9wLv4-unique") {
		t.Error("a random passphrase must not be treated as breached")
	}
}
