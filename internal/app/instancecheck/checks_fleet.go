package instancecheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/warmbly/warmbly/internal/config"
)

const docsSplitDeployment = "/development/split-deployment/"

func fleetChecks() []check {
	return []check{
		{id: "fleet_blobs_not_shared", run: checkFleetBlobsNotShared},
		{id: "fleet_infra_unreachable", run: checkFleetInfraUnreachable},
	}
}

// fleetSpansMachines reports whether live nodes sit on more than one machine.
//
// Distinct reported addresses, not a node count: replicas on one host all
// report the same address, so `--scale worker=3` on a single-machine install
// stays one machine and the checks below correctly say nothing. It is a
// deliberate under-report — one remote worker and no local node looks like one
// machine — because a false alarm on the default install is worse than a miss
// the join script already warns about.
func fleetSpansMachines(ctx context.Context, d Deps) bool {
	if d.DB == nil {
		return false
	}
	var addresses int
	err := d.DB.QueryRow(ctx, `
		SELECT count(DISTINCT address)
		FROM fleet_nodes
		WHERE active
		  AND address <> ''
		  AND last_seen_at > now() - $1::interval
	`, workerLivenessWindow.String()).Scan(&addresses)
	if err != nil {
		return false
	}
	return addresses > 1
}

// checkFleetBlobsNotShared catches the failure that looks like everything is
// fine until a send. A worker reads the message body the backend wrote; on
// another machine it has neither the disk nor the permissions, and nothing
// says so until the last step.
func checkFleetBlobsNotShared(ctx context.Context, d Deps, _ Input) *Finding {
	provider := config.BlobProvider()
	if provider != "filesystem" && provider != "fs" {
		return nil
	}
	if !fleetSpansMachines(ctx, d) {
		return nil
	}
	return result(CategoryData, SeverityError, "Blobs are on local disk and the fleet is not",
		"BLOB_PROVIDER is "+provider+", but nodes are checking in from more than one machine. A worker reads the "+
			"message body the backend wrote, so a node that does not share this filesystem cannot send at all, and the "+
			"failure only appears at the last step of a send. Move to BLOB_PROVIDER=s3 with a bucket both sides reach.",
		docsSplitDeployment)
}

// checkFleetInfraUnreachable catches the other silent one: a node is
// configured from the backend's own environment, so an address that only
// resolves here produces a node that enrols cleanly and then reaches nothing.
// It keeps heartbeating over HTTP the whole time, so the fleet looks healthy.
func checkFleetInfraUnreachable(ctx context.Context, d Deps, _ Input) *Finding {
	var bad []string
	for _, key := range []string{"NATS_URL", "REDIS"} {
		v := env(key)
		if v == "" {
			continue
		}
		if unreachableOffHost(v) {
			bad = append(bad, fmt.Sprintf("%s=%s", key, v))
		}
	}
	// renderNodeEnv falls back to APP_INTERNAL_URL, so checking only the first
	// name would pass every instance that uses the second one.
	backendKey, backendURL := "ENCRYPTED_KEYS_BACKEND_URL", env("ENCRYPTED_KEYS_BACKEND_URL")
	if backendURL == "" {
		backendKey, backendURL = "APP_INTERNAL_URL", env("APP_INTERNAL_URL")
	}
	switch {
	case backendURL == "":
		// Neither is set, so a node is told to call back on nothing. With the
		// brokered providers that is not a degraded node but one that cannot
		// open a single key.
		bad = append(bad, "ENCRYPTED_KEYS_BACKEND_URL is unset")
	case unreachableOffHost(backendURL):
		bad = append(bad, fmt.Sprintf("%s=%s", backendKey, backendURL))
	}
	if len(bad) == 0 {
		return nil
	}
	if !fleetSpansMachines(ctx, d) {
		return nil
	}
	return result(CategoryWorkers, SeverityError, "Nodes are being handed addresses they cannot reach",
		"Nodes are checking in from more than one machine, but "+strings.Join(bad, ", ")+
			" resolves only on this host. A joining node inherits these values verbatim, so it enrols, keeps "+
			"heartbeating over HTTP, and silently reaches neither the bus nor the cache. Set them to addresses "+
			"every machine in the fleet can use, then re-join the affected nodes.",
		docsSplitDeployment)
}

// unreachableOffHost reports whether a value names something only this machine
// can resolve.
//
// isLoopbackURL needs a scheme, and REDIS is sometimes written bare, so the
// host is recovered either way before it is judged. Missing that meant
// REDIS=localhost:6379 read as reachable, which is precisely the value most
// likely to be sitting there.
func unreachableOffHost(raw string) bool {
	if isLoopbackURL(raw) || isContainerInternalURL(raw) {
		return true
	}
	if strings.Contains(raw, "://") {
		return false
	}
	return isLoopbackHost(hostOnly(raw))
}

// isContainerInternalURL matches the service names the shipped compose file
// uses. They resolve inside that network and nowhere else, and they are what
// an instance that grew out of `make up` is still carrying.
func isContainerInternalURL(raw string) bool {
	host := hostOf(raw)
	if host == "" {
		// A bare host:port (REDIS is sometimes written that way) parses as a
		// path rather than a URL, so fall back to the leading label.
		host = hostOnly(strings.TrimPrefix(raw, "//"))
	}
	switch strings.ToLower(host) {
	case "nats", "redis", "postgres", "backend", "warmbly-nats", "warmbly-redis":
		return true
	}
	return false
}
