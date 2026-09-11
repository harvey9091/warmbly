#!/usr/bin/env bash
# Every COPY that reads from the build context has to name a path that exists.
#
# This exists because nothing else catches it. `go build`, `golangci-lint` and
# the whole `CI` workflow never build an image, and `build-push.yml` runs only
# on push to main, so a COPY of a file the same PR deleted is green everywhere
# until it is merged, and then main cannot publish a backend image at all. That
# is exactly what happened when the pull-based fleet change deleted
# scripts/install-worker.sh and left backend.Dockerfile copying it.
#
# Runs in a second, so it is part of `make lint` rather than a separate gate.
set -euo pipefail

cd "$(dirname "$0")/.."

red=$'\033[31m'; green=$'\033[32m'; reset=$'\033[0m'
[[ -t 1 ]] || { red=""; green=""; reset=""; }

fail=0
checked=0

# Dockerfiles anywhere in the repo except vendored trees. Each is read relative
# to the build context, which for every image here is the repository root
# except the frontends, whose context is their own directory.
while IFS= read -r df; do
  case "$df" in
    web/Dockerfile|admin/Dockerfile) context=$(dirname "$df") ;;
    tracking/Dockerfile) context=tracking ;;
    *) context="." ;;
  esac

  # Only COPY lines that read from the context. `--from=` reads from an earlier
  # stage or an image, which is not ours to resolve.
  while IFS= read -r line; do
    [[ "$line" == *"--from="* ]] && continue

    # Strip the COPY keyword and any remaining flags, then drop the final
    # argument, which is the destination inside the image.
    args=${line#COPY }
    srcs=()
    for tok in $args; do
      [[ "$tok" == --* ]] && continue
      srcs+=("$tok")
    done
    (( ${#srcs[@]} >= 2 )) || continue
    unset 'srcs[${#srcs[@]}-1]'

    for src in "${srcs[@]}"; do
      # A source may be a glob (Cargo.lock*), and a glob matching nothing is
      # legal in Docker only when another source in the same COPY matches, so
      # a bare glob is not something this can judge. Skip them; a literal path
      # is what the bug looked like and what this is here to catch.
      [[ "$src" == *"*"* || "$src" == *"?"* || "$src" == *"["* ]] && continue
      checked=$((checked + 1))
      if [[ ! -e "$context/$src" ]]; then
        echo "${red}✗${reset} $df copies $src, which does not exist (context: $context)"
        fail=1
      fi
    done
  done < <(grep -E '^[[:space:]]*COPY[[:space:]]' "$df" | sed -E 's/^[[:space:]]*//')
# '*Dockerfile*', not 'Dockerfile*': every Go service's file is named
# <service>.Dockerfile, so anchoring the pattern at the start silently skips
# the ten that matter and leaves the check reporting success over three files.
done < <(find . -iname '*Dockerfile*' -not -path './.git/*' -not -path '*/node_modules/*' \
           -not -path '*/target/*' -printf '%P\n' | sort)

if (( fail )); then
  echo
  echo "${red}A Dockerfile copies a path that is not in the repository.${reset}"
  echo "Nothing else fails on this until it is on main and the image build breaks."
  exit 1
fi

echo "${green}✓${reset} check-dockerfiles: $checked COPY sources all present."
