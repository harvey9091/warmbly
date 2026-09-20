package models

import "testing"

func TestWarmupPartnerCandidateStarvation(t *testing.T) {
	cases := []struct {
		name           string
		sent, received int
		want           float64
	}{
		{"sends nothing, owes nothing", 0, 3, 0},
		{"heard back from nobody", 20, 0, 1},
		{"heard back from one in twenty", 20, 1, 0.95},
		{"in balance", 10, 10, 0},
		{"received more than it sent", 5, 12, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := WarmupPartnerCandidate{Sent7d: tc.sent, Received7d: tc.received}
			if got := c.Starvation(); got != tc.want {
				t.Fatalf("Starvation() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWarmupPoolReturnsToMirrorsTheBorrow(t *testing.T) {
	for _, pool := range []string{"free", "premium"} {
		borrowFrom, borrows := WarmupPoolBorrowsFrom(pool)
		returnTo, returns := WarmupPoolReturnsTo(pool)
		if borrows == returns {
			t.Fatalf("%s both borrows and returns, or neither", pool)
		}
		if borrows {
			if back, ok := WarmupPoolReturnsTo(borrowFrom); !ok || back != pool {
				t.Fatalf("%s borrows from %s, which returns to %q", pool, borrowFrom, back)
			}
		}
		if returns {
			if from, ok := WarmupPoolBorrowsFrom(returnTo); !ok || from != pool {
				t.Fatalf("%s returns to %s, which borrows from %q", pool, returnTo, from)
			}
		}
	}
}
