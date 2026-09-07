package auth

import "github.com/warmbly/warmbly/internal/models"

type AuthData struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	Turnstile string `json:"turnstile"`
	// ReferralCode is the optional ?ref= code the signup arrived with. Carried
	// to RegistrationConfirm so the new org can be attributed to its referrer.
	ReferralCode string `json:"referral_code"`
	// Invite is the team-invitation token the signup arrived with. It is what
	// lets an invited person create an account under DISABLE_REGISTRATION=invite_only.
	Invite string `json:"invite"`
	// Acquisition is where the visitor came from, read by the dashboard from
	// the signup URL's query string. Recorded once on the new organization and
	// never updated. Absent for a direct visit, which is most signups.
	Acquisition models.OrgAcquisition `json:"acquisition"`
}

type ConfirmData struct {
	Session   string `json:"session"`
	Code      string `json:"code"`
	Turnstile string `json:"turnstile"`
}

type ResetPasswordStart struct {
	Email     string `json:"email"`
	Turnstile string `json:"turnstile"`
}

type ResetPasswordConfirm struct {
	Session   string `json:"session"`
	Password  string `json:"password"`
	Turnstile string `json:"turnstile"`
}

type ChangePassword struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}
