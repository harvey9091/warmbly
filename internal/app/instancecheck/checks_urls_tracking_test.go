package instancecheck

import "testing"

// The exact misconfiguration this exists for: a tracking host on the brand's
// own registered domain, where a URL blocklisting earned by one workspace's
// complaints reaches the site and the platform mail too.
func TestTrackingDomainSharesBrand(t *testing.T) {
	t.Setenv("APP_URL", "https://app.warmbly.com")
	t.Setenv("API_PUBLIC_URL", "https://api.warmbly.com")

	t.Setenv("TRACKING_DOMAIN", "track.warmbly.com")
	if f := checkTrackingDomainSharesBrand(t.Context(), Deps{}, Input{}); f == nil {
		t.Error("a tracking host on the brand's registered domain was not reported")
	}

	// A separate registered domain is the whole point, and must stay silent.
	t.Setenv("TRACKING_DOMAIN", "t.emberbly.com")
	if f := checkTrackingDomainSharesBrand(t.Context(), Deps{}, Input{}); f != nil {
		t.Errorf("a separate tracking domain was reported: %s", f.Message)
	}

	// Unset is a different finding's job, and localhost is development.
	for _, v := range []string{"", "localhost:3000"} {
		t.Setenv("TRACKING_DOMAIN", v)
		if f := checkTrackingDomainSharesBrand(t.Context(), Deps{}, Input{}); f != nil {
			t.Errorf("TRACKING_DOMAIN=%q was reported: %s", v, f.Message)
		}
	}
}
