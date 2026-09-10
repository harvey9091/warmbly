package instancecheck

import "testing"

// The values most likely to be sitting in a config that grew out of `make up`,
// plus the bare host:port form REDIS is sometimes written in. Missing that form
// meant the check passed the one value it most needed to catch.
func TestUnreachableOffHost(t *testing.T) {
	unreachable := []string{
		"nats://nats:4222",
		"tls://token@nats:4222",
		"redis://redis:6379",
		"rediss://:pass@redis:6379",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"localhost:6379",
		"127.0.0.1:6379",
		"redis:6379",
	}
	for _, v := range unreachable {
		if !unreachableOffHost(v) {
			t.Errorf("%q reported reachable from another machine", v)
		}
	}

	reachable := []string{
		"tls://token@bus.example.com:4222",
		"rediss://:pass@bus.example.com:6380",
		"https://api.example.com",
		"bus.example.com:4222",
		"nats://10.0.1.5:4222",
	}
	for _, v := range reachable {
		if unreachableOffHost(v) {
			t.Errorf("%q reported unreachable", v)
		}
	}
}
