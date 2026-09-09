package models

import "time"

// Admin settings keys for the fleet. Both live in `admin_settings` so every
// backend replica agrees and the values survive a restart.
const (
	// FleetSettingsKeyRelease holds the resolved release the fleet should be
	// running (FleetReleaseState).
	FleetSettingsKeyRelease = "fleet.release"
	// FleetSettingsKeyJoinToken holds the bcrypt-style hash of the instance
	// join token. The token itself is shown once, at issue time, and never
	// stored.
	FleetSettingsKeyJoinToken = "fleet.join_token"
)

// ReleaseChannelStable and friends name which GitHub releases the fleet
// follows.
const (
	FleetChannelStable = "stable"
	FleetChannelDev    = "dev"
	// FleetChannelPinned freezes the fleet at whatever Tag currently says.
	// Nodes still self-update TO that tag; they just stop following new ones.
	FleetChannelPinned = "pinned"
)

// FleetReleaseState is the answer to "what version should my nodes be running".
//
// One tag covers every role: worker and consumer ship from the same repository
// and the same release, so a node needs the tag and already knows its own
// image name. Keeping it to one value is what makes "everything is on the same
// version" checkable at a glance instead of a per-role matrix.
type FleetReleaseState struct {
	Channel string `json:"channel"`
	// Tag is the resolved release, e.g. "v1.4.2". Empty means nothing has been
	// resolved yet, which every consumer of this must read as "no opinion"
	// rather than "downgrade to nothing".
	Tag        string    `json:"tag"`
	ResolvedAt time.Time `json:"resolved_at"`
	// Source records how Tag was set, so an operator can tell an automatic
	// resolution from a manual pin.
	Source string `json:"source,omitempty"`
}

// DesiredVersion is the tag nodes should converge on, or "" when the control
// plane has no opinion.
func (s *FleetReleaseState) DesiredVersion() string {
	if s == nil {
		return ""
	}
	return s.Tag
}
