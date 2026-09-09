// Package fleetnode is the control-plane half of the pull-based fleet.
//
// A node joins by running one command with the instance join token, then
// heartbeats forever. It is never reached into: everything the control plane
// wants a node to do comes back in the heartbeat reply, which today is exactly
// one instruction — what version to be running.
package fleetnode

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/crypt"
	"github.com/warmbly/warmbly/internal/repository"
)

var (
	// ErrNoJoinToken means no token has ever been issued, so nothing may join.
	// Deliberately distinct from a wrong token: an instance with no token is a
	// setup problem, not an attack.
	ErrNoJoinToken = errors.New("no join token has been issued for this instance")
	ErrBadToken    = errors.New("join token is not valid")
	ErrBadRole     = errors.New("unknown node role")
	// ErrRoleChanged is a node claiming an id that is already registered under
	// the other role. Silently accepting it would leave a worker's mailboxes
	// assigned to a machine that has stopped doing worker work.
	ErrRoleChanged = errors.New("that node id is already registered under a different role")
)

type Service struct {
	nodes    repository.FleetNodeRepository
	workers  repository.WorkerRepository
	settings repository.FleetSettingsRepository
}

func New(
	nodes repository.FleetNodeRepository,
	workers repository.WorkerRepository,
	settings repository.FleetSettingsRepository,
) *Service {
	return &Service{nodes: nodes, workers: workers, settings: settings}
}

// IssueJoinToken mints a new instance join token, stores only its hash, and
// returns the plaintext. The caller must show it once: it cannot be recovered.
//
// Issuing replaces any previous token, which is also how you revoke one.
// Existing nodes are unaffected — they are already enrolled, and the token
// only gates joining.
func (s *Service) IssueJoinToken(ctx context.Context) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.settings.SetJoinTokenHash(ctx, crypt.SHA256(token)); err != nil {
		return "", err
	}
	return token, nil
}

// VerifyJoinToken checks a presented token in constant time.
func (s *Service) VerifyJoinToken(ctx context.Context, token string) error {
	want, err := s.settings.GetJoinTokenHash(ctx)
	if err != nil {
		return err
	}
	if want == "" {
		return ErrNoJoinToken
	}
	got := crypt.SHA256(strings.TrimSpace(token))
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return ErrBadToken
	}
	return nil
}

// Heartbeat records a beat and answers with what the node should be running.
//
// Registration is the first heartbeat: there is no separate create step, so a
// node rebuilt from scratch simply reappears under the same id. A node that
// declares itself a worker also gets its placement row, because placement
// reads `workers` and a node with no row there would be invisible to it.
func (s *Service) Heartbeat(ctx context.Context, beat models.NodeHeartbeat) (*models.NodeHeartbeatReply, error) {
	if !beat.Role.Valid() {
		return nil, ErrBadRole
	}
	if beat.NodeID == uuid.Nil {
		return nil, errors.New("node_id required")
	}

	// A node may not change what it does under the same id. Checked before the
	// upsert so the row is never half-migrated between roles.
	if existing, err := s.nodes.Get(ctx, beat.NodeID); err == nil && existing != nil && existing.Role != beat.Role {
		return nil, ErrRoleChanged
	}

	if beat.Stopping {
		// The farewell beat. Go inactive at once rather than staying selectable
		// until the beat ages out; placement must not hand work to a process
		// that has already gone.
		if err := s.nodes.Deactivate(ctx, beat.NodeID); err != nil {
			return nil, err
		}
		return &models.NodeHeartbeatReply{LivenessSeconds: int(models.NodeLivenessWindow.Seconds())}, nil
	}

	if err := s.nodes.UpsertOnHeartbeat(ctx, beat); err != nil {
		return nil, err
	}
	if beat.Role == models.NodeRoleWorker && s.workers != nil {
		if err := s.workers.EnsureWorkerRow(ctx, beat.NodeID); err != nil {
			return nil, err
		}
	}

	reply := &models.NodeHeartbeatReply{
		LivenessSeconds: int(models.NodeLivenessWindow.Seconds()),
	}
	reply.DesiredVersion = s.desiredVersion(ctx, beat.NodeID)
	return reply, nil
}

// desiredVersion resolves what this node should run: its own pin if it has
// one, otherwise the fleet-wide resolved release.
//
// Any failure answers "" — no opinion. A node that cannot be told what to run
// must keep running what it has, because the alternative is a control-plane
// hiccup rolling the whole fleet.
func (s *Service) desiredVersion(ctx context.Context, nodeID uuid.UUID) string {
	if node, err := s.nodes.Get(ctx, nodeID); err == nil && node != nil && node.PinnedVersion != "" {
		return node.PinnedVersion
	}
	state, err := s.settings.GetRelease(ctx)
	if err != nil {
		return ""
	}
	return state.DesiredVersion()
}

// List returns the fleet, with each node's resolved target attached so a
// caller can see at a glance which machines are behind.
func (s *Service) List(ctx context.Context, role models.NodeRole) ([]models.FleetNode, error) {
	nodes, err := s.nodes.List(ctx, role)
	if err != nil {
		return nil, err
	}
	state, err := s.settings.GetRelease(ctx)
	if err != nil {
		return nil, err
	}
	fleetTarget := state.DesiredVersion()
	for i := range nodes {
		if nodes[i].PinnedVersion != "" {
			nodes[i].DesiredVersion = nodes[i].PinnedVersion
			continue
		}
		nodes[i].DesiredVersion = fleetTarget
	}
	return nodes, nil
}

// Get returns one node with its resolved target attached.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*models.FleetNode, error) {
	node, err := s.nodes.Get(ctx, id)
	if err != nil || node == nil {
		return nil, err
	}
	if node.PinnedVersion != "" {
		node.DesiredVersion = node.PinnedVersion
		return node, nil
	}
	state, err := s.settings.GetRelease(ctx)
	if err != nil {
		return nil, err
	}
	node.DesiredVersion = state.DesiredVersion()
	return node, nil
}
