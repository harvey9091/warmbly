package auth

import "time"

const (
	SessionTTL   = 10 * time.Minute
	AuthAttempts = 3

	// LoginFailureLimit and LoginFailureTTL bound password guessing against one
	// account, which the per-IP limiter cannot: a guesser with a botnet spends a
	// fresh 60-request budget per source address while the account it is aimed
	// at counts nothing.
	//
	// The figure is the 100 per hour CASA 1.1.1 names, not something tighter. A
	// counter keyed on an address is a lockout anyone can trigger by typing a
	// wrong password at somebody else's account, so the number has to sit above
	// what a person hits by mistake and below what a guesser needs. Ten was
	// both: within reach of a shared office retyping a password, and cheap to
	// aim at a known address.
	//
	// The count is also cleared by a correct password and by a completed
	// password reset, so someone locked out has two ways back that do not
	// involve waiting, and an attacker cannot hold the lock open.
	LoginFailureLimit = 100
	LoginFailureTTL   = 1 * time.Hour

	AuthSessionTTL   = 10 * time.Minute
	AuthEmailTTL     = 30 * time.Minute
	AuthEmailLimit   = 5
	PasswordResetTTL = 1 * time.Hour

	PasswordResetLimit    = 2
	PasswordResetLimitTTL = 4 * time.Hour

	// KnownDeviceTTL is how long a device stays exempt from the login code
	// under AUTH_LOGIN_CODE=new_device.
	KnownDeviceTTL = 90 * 24 * time.Hour
)

// Email send-budget flows. Separate keys so registration traffic cannot exhaust
// a user's login budget.
const (
	emailFlowLogin        = "login"
	emailFlowRegistration = "registration"
)
