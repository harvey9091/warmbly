package form

import "testing"

func TestValidFormsDomain(t *testing.T) {
	valid := []string{
		"forms.acme.com",
		"go.acme.co.uk",
		"a-b.example.com",
		"x1.y2.example.io",
	}
	for _, v := range valid {
		if !validFormsDomain(v) {
			t.Errorf("validFormsDomain(%q) = false, want true", v)
		}
	}

	invalid := []string{
		"",                // empty
		"localhost",       // no dot
		"-lead.acme.com",  // label starts with a hyphen
		"lead-.acme.com",  // label ends with a hyphen
		"acme..com",       // empty label
		"acme.com/forms",  // path is not part of a host
		"forms acme.com",  // space
		"forms.acme.com:", // stray separator
	}
	for _, v := range invalid {
		if validFormsDomain(v) {
			t.Errorf("validFormsDomain(%q) = true, want false", v)
		}
	}
}
