package models

// EmailRiskBand classifies a mailbox by reputation risk. The rebalancer
// derives this from WarmupHealthState and writes it into
// email_accounts.risk_band; nothing else should set it. It drives warmup
// partner selection and per-mailbox pacing, not worker placement: a mailbox
// landing in spam does not contaminate the machine it sends from, because the
// machine is not the sending identity.
type EmailRiskBand string

const (
	EmailRiskBandClean      EmailRiskBand = "clean"
	EmailRiskBandRisky      EmailRiskBand = "risky"
	EmailRiskBandQuarantine EmailRiskBand = "quarantine"
)

// RiskBandFromHealth maps the warmup health state machine into the simpler
// three-bucket risk_band. The mapping is one-way
// (collapses watch/throttled/quarantined into the recovery pool) — the
// reverse direction is meaningless.
//
//	healthy           → clean
//	watch, throttled  → risky      (degraded but still sending)
//	quarantined,      → quarantine (sending stopped or close to it)
//	blocked
//
// Any state not covered (e.g. a row with NULL warmup state) defaults to
// clean — assume innocent until proven otherwise.
func RiskBandFromHealth(s WarmupHealthState) EmailRiskBand {
	switch s {
	case WarmupHealthWatch, WarmupHealthThrottled:
		return EmailRiskBandRisky
	case WarmupHealthQuarantined, WarmupHealthBlocked:
		return EmailRiskBandQuarantine
	default:
		return EmailRiskBandClean
	}
}
