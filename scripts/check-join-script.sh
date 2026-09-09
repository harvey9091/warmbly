#!/bin/sh
# Everything CI should know about the fleet join script.
#
# The script is served verbatim from the backend at GET /join.sh and is what a
# stranger pipes into a root shell to add a machine, which makes it the
# highest-consequence file in the repo that is not Go. Nothing else covered it,
# and that is how a systemd unit that could never start, an env file with a
# stray JSON fragment in it, and a state directory the node could not write all
# reached the branch at once.
#
# What is checked:
#   - POSIX parse under dash, which is /bin/sh on Debian and Ubuntu
#   - shellcheck, in sh mode
#   - --help exits 0 and says something
#   - the generated systemd unit is ONE ExecStart line with the image as a
#     systemd variable, not a command substitution systemd would never expand
#   - the mount list always includes the agent directory
#   - a relative BLOB_FS_ROOT is refused rather than mounted
set -eu

SCRIPT="internal/api/handler/nodescript/join.sh"
fail() { printf 'check-join-script: %s\n' "$*" >&2; exit 1; }
ok()   { printf '  ok  %s\n' "$*"; }

[ -f "$SCRIPT" ] || fail "$SCRIPT not found (run from the repository root)"

# POSIX parse. sh -n under a non-POSIX shell proves nothing about dash, which
# is /bin/sh on Debian and Ubuntu, so dash is used when it is there.
parse_check() {
  if command -v dash >/dev/null 2>&1; then
    dash -n "$1" || fail "dash -n failed on $1"
  else
    sh -n "$1" || fail "sh -n failed on $1"
  fi
}

parse_check "$SCRIPT"
# The checker too: its own shellcheck-disable directives are load-bearing.
parse_check "$0"
if command -v dash >/dev/null 2>&1; then
  ok "dash -n (script and checker)"
else
  printf '  --  dash not installed; parsed with sh -n instead\n'
fi

if command -v shellcheck >/dev/null 2>&1; then
  shellcheck -s sh "$SCRIPT" || fail "shellcheck failed on $SCRIPT"
  shellcheck -s sh "$0" || fail "shellcheck failed on $0"
  ok "shellcheck -s sh (script and checker)"
else
  printf '  --  shellcheck not installed; skipped\n'
fi

out=$(sh "$SCRIPT" --help) || fail "--help exited non-zero"
printf '%s' "$out" | grep -q -- "--token" || fail "--help does not document --token"
ok "--help"

# Everything below asserts on what the script RENDERS, never on its source
# text. A previous version of this file checked a heredoc copied in here, which
# meant putting the original `$(cat ...)` bug back left it passing green.
# The one non-default environment the unit is rendered under. Named for what
# it is rather than dressed up as a list: sh has no clean way to iterate blocks
# that themselves contain newlines, so a third variant means adding it to the
# `for` below by hand.
FS_BLOB_ENV='BLOB_PROVIDER=fs
BLOB_FS_ROOT=/var/lib/warmbly/blobs'

# NODE_ENV is forced empty rather than inherited: this repo exports NODE_ENV in
# several trees, and an inherited value would render a unit this check did not
# choose, or fail validate_blob_root for an unrelated reason.
unit=$(NODE_ENV="" sh "$SCRIPT" --print-unit) || fail "--print-unit failed"

# shellcheck disable=SC2016  # the pattern is literal on purpose; it must not expand
printf '%s\n' "$unit" | grep -q 'ExecStart=.*\${WARMBLY_IMAGE_REF}$' \
  || fail "ExecStart must end with the systemd variable \${WARMBLY_IMAGE_REF}"
# shellcheck disable=SC2016  # literal on purpose
if printf '%s\n' "$unit" | grep -q 'ExecStart=.*\$('; then
  fail "ExecStart contains a command substitution; systemd never expands one"
fi
[ "$(printf '%s\n' "$unit" | grep -c '^ExecStart=')" = "1" ] \
  || fail "ExecStart must be exactly one line"
printf '%s\n' "$unit" | grep -q '^EnvironmentFile=/var/lib/warmbly/image-ref$' \
  || fail "image-ref must stay in the root-owned state dir; the node must not be able to rewrite it"
printf '%s\n' "$unit" | grep -q 'ExecStart=.*-v /var/lib/warmbly/node:/var/lib/warmbly/node' \
  || fail "the agent directory must always be mounted, or auto-update stops silently"
ok "rendered unit (no blob mount)"

# With local blobs the root has to be mounted too, and the line must still be
# one line: a multi-line mount list is how the continuation collapsed before.
unit=$(NODE_ENV="$FS_BLOB_ENV" sh "$SCRIPT" --print-unit) || fail "--print-unit with blobs failed"
printf '%s\n' "$unit" | grep -q 'ExecStart=.*-v /var/lib/warmbly/blobs:/var/lib/warmbly/blobs' \
  || fail "BLOB_FS_ROOT must be mounted (the fs alias counts as filesystem)"
[ "$(printf '%s\n' "$unit" | grep -c '^ExecStart=')" = "1" ] \
  || fail "ExecStart must stay one line when a blob mount is added"
ok "rendered unit (fs alias + blob mount)"

# A relative root is refused rather than rendered into a mount docker rejects.
if NODE_ENV="BLOB_PROVIDER=filesystem
BLOB_FS_ROOT=data/blobs" sh "$SCRIPT" --print-unit >/dev/null 2>&1; then
  fail "a relative BLOB_FS_ROOT must be refused, not mounted"
fi
ok "relative BLOB_FS_ROOT refused"

# NO EnvironmentFile may point into the node-writable mount. Asserting only
# that the right one exists is not enough: an extra one under AGENT_DIR would
# let the container choose the image root's `docker run --network host` runs.
# Checked in every render, not just the default one. EnvironmentFile does not
# vary with NODE_ENV today, but the point of this assertion is what someone
# changes tomorrow, and "today it is redundant" is exactly the reasoning that
# already dropped this guard once. One extra subshell is a fair price.
for variant_env in "" "$FS_BLOB_ENV"; do
  v_unit=$(NODE_ENV="$variant_env" sh "$SCRIPT" --print-unit) || fail "--print-unit failed"
  if printf '%s\n' "$v_unit" | grep '^EnvironmentFile=' | grep -q '/var/lib/warmbly/node'; then
    fail "an EnvironmentFile points into the node-writable mount; the node could choose the image root runs"
  fi
done
ok "no EnvironmentFile is node-writable (every render)"

# Two invariants that leave no trace in the rendered unit and so cannot be
# caught above: both were real defects, so they are asserted at their call
# sites. Comments are stripped and the call is matched in command position, so
# a commented-out call fails while reformatting does not.
# body_of prints one function's body. Tolerant about the definition's spacing,
# and loud when the function is not found: an empty body would otherwise fail
# every assertion below with a message about the wrong thing.
body_of() {
  out=$(awk -v fn="$1" '
    $0 ~ "^" fn "[ \t]*\\([ \t]*\\)[ \t]*{" { inside = 1 }
    inside { print }
    inside && $0 == "}" { exit }' "$SCRIPT")
  [ -n "$out" ] || fail "function $1() not found in $SCRIPT"
  printf '%s\n' "$out"
}

# Match the call lines themselves, not any line mentioning the word: "enrol"
# also appears inside the word "enrolment" in a comment.
# Captured before asserting: body_of fails when the function is missing, and a
# pipeline would run it in a subshell where that failure is lost and the
# misleading assertion message wins.
main_body=$(body_of main)
printf '%s\n' "$main_body" | awk '
  $1 == "enrol"              { e = NR }
  $1 == "validate_blob_root" { v = NR }
  $1 == "write_config"       { w = NR }
  END { exit !(e && v && w && e < v && v < w) }' \
  || fail "main must call validate_blob_root between enrol and write_config"
ok "config is validated before anything is written"

# Field match on the first word, which cannot be fooled by the name appearing
# inside a string or a comment. That does mean the call has to stay a
# standalone statement; join.sh says so at the call site. A looser regex was
# tried and was satisfied by `warn "... ensure_blob_root ..."`, which is a much
# worse failure than a reformat that reports itself clearly.
install_body=$(body_of install_units)
printf '%s\n' "$install_body" | awk '$1 == "ensure_blob_root" { found = 1 } END { exit !found }' \
  || fail "install_units must call ensure_blob_root as a standalone statement, or a filesystem-blob node restart-loops"
ok "blob root is prepared before the unit is installed"

printf 'check-join-script: all checks passed\n'
