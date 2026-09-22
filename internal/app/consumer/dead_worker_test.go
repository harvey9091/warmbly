package jobs

import "testing"

func TestWorkerRecoveryOutcomeState(t *testing.T) {
	tests := []struct {
		name    string
		outcome workerRecoveryOutcome
		want    string
	}{
		{name: "complete", outcome: workerRecoveryOutcome{Reassigned: 3}, want: "complete"},
		{name: "partial", outcome: workerRecoveryOutcome{Reassigned: 2, Stranded: 1}, want: "partial"},
		{name: "failed", outcome: workerRecoveryOutcome{Stranded: 3}, want: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.outcome.state(); got != tt.want {
				t.Fatalf("state() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkerRecoveryOutcomeFailureSummary(t *testing.T) {
	outcome := workerRecoveryOutcome{FailureReasons: map[string]int{
		"no_eligible_worker": 2,
		"move_failed":        1,
	}}

	if got, want := outcome.failureSummary(), "move failed: 1, no eligible worker: 2"; got != want {
		t.Fatalf("failureSummary() = %q, want %q", got, want)
	}
}
