package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds one git invocation so a hung remote cannot pin a job.
const gitTimeout = 3 * time.Minute

type git struct {
	dir    string
	remote string
}

// run executes git in the checkout. safe.directory covers the compose case,
// where the sidecar runs as root against a checkout the operator owns.
func (g git) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	full := append([]string{"-c", "safe.directory=*", "-C", g.dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func (g git) head(ctx context.Context) (string, error) {
	return g.run(ctx, "rev-parse", "HEAD")
}

// branch returns the current branch, or "" when HEAD is detached.
func (g git) branch(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if out == "HEAD" {
		return "", nil
	}
	return out, nil
}

func (g git) describe(ctx context.Context) string {
	out, err := g.run(ctx, "describe", "--tags", "--always", "--dirty")
	if err != nil {
		return ""
	}
	return out
}

func (g git) dirty(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func (g git) fetch(ctx context.Context) error {
	_, err := g.run(ctx, "fetch", "--tags", "--prune", g.remote)
	return err
}

func (g git) remoteHead(ctx context.Context, branch string) (string, error) {
	return g.run(ctx, "rev-parse", g.remote+"/"+branch)
}

// behind counts commits on the remote branch that HEAD does not have.
func (g git) behind(ctx context.Context, branch string) (int, error) {
	out, err := g.run(ctx, "rev-list", "--count", "HEAD.."+g.remote+"/"+branch)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

func (g git) pull(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "merge", "--ff-only", g.remote+"/"+branch)
	return err
}

func (g git) checkoutTag(ctx context.Context, tag string) error {
	_, err := g.run(ctx, "checkout", "--detach", "refs/tags/"+tag)
	return err
}

func (g git) tagExists(ctx context.Context, tag string) bool {
	_, err := g.run(ctx, "rev-parse", "--verify", "-q", "refs/tags/"+tag)
	return err == nil
}

// inspect reads the checkout state without touching the network. fetch runs
// separately so a slow remote never delays a status answer.
func (g git) inspect(ctx context.Context) (*Checkout, error) {
	head, err := g.head(ctx)
	if err != nil {
		return nil, err
	}
	branch, err := g.branch(ctx)
	if err != nil {
		return nil, err
	}
	c := &Checkout{Commit: head, Branch: branch, Detached: branch == "", Describe: g.describe(ctx)}
	if dirty, err := g.dirty(ctx); err == nil {
		c.Dirty = dirty
	}
	if branch != "" {
		if remote, err := g.remoteHead(ctx, branch); err == nil {
			c.RemoteCommit = remote
		}
		if n, err := g.behind(ctx, branch); err == nil {
			c.Behind = n
		}
	}
	return c, nil
}
