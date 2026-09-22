package models

import (
	"testing"

	"github.com/warmbly/warmbly/internal/config"
)

func TestWarmupMailRetentionDays(t *testing.T) {
	cases := []struct {
		name     string
		mailbox  int
		instance int
		want     int
	}{
		{"unset row follows the instance", 0, 45, 45},
		{"own window wins", 14, 45, 14},
		{"instance below the floor resolves to the default", 0, 0, config.WarmupMailRetentionDaysDefault},
		{"garbage on the row follows the instance", 1, 45, 45},
		{"garbage on the row and no instance value resolves to the default", -3, 0, config.WarmupMailRetentionDaysDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Email{WarmupRetentionDays: tc.mailbox}
			if got := e.WarmupMailRetentionDays(tc.instance); got != tc.want {
				t.Fatalf("WarmupMailRetentionDays(%d) = %d, want %d", tc.instance, got, tc.want)
			}
		})
	}
}

func TestValidWarmupRetentionDays(t *testing.T) {
	for _, ok := range []int{0, config.WarmupMailRetentionDaysMin, 30, config.RetentionDaysMax} {
		if !ValidWarmupRetentionDays(ok) {
			t.Errorf("%d should be an accepted window", ok)
		}
	}
	for _, bad := range []int{-1, 1, config.WarmupMailRetentionDaysMin - 1, config.RetentionDaysMax + 1} {
		if ValidWarmupRetentionDays(bad) {
			t.Errorf("%d should be refused", bad)
		}
	}
}
