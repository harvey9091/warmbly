package config

import "testing"

func TestMailvendorSandboxURLOnlyInDevelopment(t *testing.T) {
	t.Setenv("MAILVENDOR_SANDBOX_URL", "http://127.0.0.1:18099/")
	for env, want := range map[string]string{"": "http://127.0.0.1:18099", "dev": "http://127.0.0.1:18099", "prod": "", "production": "", "staging": ""} {
		t.Setenv("APP_ENV", env)
		if got := MailvendorSandboxURL(); got != want {
			t.Errorf("APP_ENV=%q: got %q, want %q", env, got, want)
		}
	}
}
