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
	for _, key := range []string{"NATS_URL", "REDIS", "ENCRYPTED_KEYS_BACKEND_URL"} {
		v := env(key)
		if v == "" {
			continue
		}
		if isLoopbackURL(v) || isContainerInternalURL(v) {
			bad = append(bad, fmt.Sprintf("%s=%s", key, v))
		}
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
