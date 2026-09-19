#!/usr/bin/env bash
# Generate the CASA dependency-scan artifacts.
#
# Everything here is read-only against the tree: it runs scanners and writes
# their output under compliance/casa/artifacts/. Two artifacts cannot come from
# this repository and are attached by hand before submission: the Qualys SSL
# Labs report per hostname, and the authenticated Burp Suite scan.
#
# A scanner that is not installed is recorded as not run rather than silently
# skipped. An evidence pack with a gap in it is honest; one that hides the gap
# is not.
set -uo pipefail

cd "$(dirname "$0")/.."
OUT="compliance/casa/artifacts"
mkdir -p "$OUT"
STAMP="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
COMMIT="$(git rev-parse HEAD)"

header() {
    printf '# %s\n\nGenerated: %s\nCommit: %s\n\n' "$1" "$STAMP" "$COMMIT"
}

echo "==> govulncheck (default build)"
{
    header "govulncheck, default build"
    go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 || true
} >"$OUT/govulncheck.txt"

echo "==> govulncheck (kafka build)"
{
    header "govulncheck, kafka build variant"
    printf 'The Avro codec is behind a build tag, so the default scan never compiles it.\n\n'
    go run golang.org/x/vuln/cmd/govulncheck@latest -tags kafka ./... 2>&1 || true
} >"$OUT/govulncheck-kafka.txt"

echo "==> pnpm audit per tree"
{
    header "Node production dependencies"
    for tree in web admin site docs forms; do
        printf '\n## %s\n\n```\n' "$tree"
        (cd "$tree" && pnpm audit --audit-level=high --prod 2>&1) || true
        printf '```\n'
    done
} >"$OUT/node-audit.txt"

echo "==> cargo audit"
{
    header "Rust dependencies"
    if command -v cargo-audit >/dev/null 2>&1; then
        (cd tracking && cargo audit 2>&1) || true
    else
        printf 'cargo-audit is not installed on this machine, so this scan did not run here.\n'
        printf 'CI runs it on every dependency change: see the rust job in .github/workflows/security.yml.\n'
    fi
} >"$OUT/rust-audit.txt"

echo "==> mix hex.audit"
{
    header "Elixir dependencies"
    if command -v mix >/dev/null 2>&1 && [ -d realtime/deps ]; then
        (cd realtime && mix hex.audit 2>&1) || true
    else
        printf 'mix is unavailable or dependencies are not fetched, so this scan did not run here.\n'
        printf 'Run: cd realtime && mix deps.get && mix hex.audit\n'
    fi
} >"$OUT/elixir-audit.txt"

echo "==> trivy"
{
    header "Trivy filesystem scan"
    if command -v trivy >/dev/null 2>&1; then
        trivy fs --scanners vuln --severity HIGH,CRITICAL . 2>&1 || true
    else
        printf 'trivy is not installed on this machine, so this scan did not run here.\n'
        printf 'CI runs it on every dependency change: see the trivy job in .github/workflows/security.yml.\n'
    fi
} >"$OUT/trivy.txt"

echo
echo "Wrote:"
ls -1 "$OUT"
echo
echo "Still to attach by hand before submission:"
echo "  - Qualys SSL Labs report per hostname in compliance/casa/scope.md"
echo "  - Authenticated Burp Suite scan using the ADA scan configuration"
