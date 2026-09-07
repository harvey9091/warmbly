package updates

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, running  string
		want, comparable bool
	}{
		{"v1.5.0", "v1.4.0", true, true},
		{"v1.4.0", "v1.4.0", false, true},
		{"v1.4.0", "v1.5.0", false, true},
		{"v1.4.1", "v1.4.0-3-gabc1234", true, true},
		{"v1.4.0", "v1.4.0-3-gabc1234", false, true},
		{"v1.5.0", "v1.5.0-rc.1", true, true},
		{"v1.5.0-rc.2", "v1.5.0-rc.1", true, true},
		{"v1.5.0", "v1.5.0-rc.1-2-gabc1234", true, true},
		{"v1.5.0-rc.1-3-gabc1234", "v1.5.0-rc.1-2-gabc1234", true, true},
		{"v1.5.0-rc.1", "v1.5.0-rc.1-2-gabc1234", false, true},
		{"v2.0.0", "1.9.9", true, true},
		{"v1.5.0", "dev", false, false},
		{"v1.5.0", "abc1234", false, false},
		{"", "v1.5.0", false, false},
		{"v1.5.0", "v1.4.0-dirty", true, true},
		// An overflowing commit distance is not a version at all.
		{"v1.5.0", "v1.4.0-99999999999999999999-gabc1234", false, false},
	}
	for _, c := range cases {
		got, ok := newer(c.latest, c.running)
		if got != c.want || ok != c.comparable {
			t.Errorf("newer(%q, %q) = (%v, %v), want (%v, %v)", c.latest, c.running, got, ok, c.want, c.comparable)
		}
	}
}
