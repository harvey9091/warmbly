#!/usr/bin/env sh
# The forms design engine is mirrored byte-identical across the two React
# trees (the builder canvas and the hosted page must render the same). This
# fails lint when the copies drift; fix by copying the edited file over the
# stale one.
set -e
status=0
for pair in \
    "forms/src/designCore.ts web/src/components/app/forms/designCore.ts" \
    "forms/src/form-theme.css web/src/components/app/forms/form-theme.css"; do
    set -- $pair
    if ! diff -q "$1" "$2" >/dev/null 2>&1; then
        echo "forms mirror drift: $1 and $2 differ (copy the edited one over the other)" >&2
        status=1
    fi
done
exit $status
