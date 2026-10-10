#!/usr/bin/env bash
# Runs all audio doctor checks in order and prints a summary. Read-only.
# Usage: run-all.sh [--pw-dump FILE] [--since TIME] [--no-probe] [--pw-link FILE]
# Exit: 0 all OK, 1 at least one FAULT, 2 no FAULT but a SKIP or ERROR.
set -euo pipefail

here=$(dirname "$0")
ok=0
fault=0
skip=0
error=0

for check in "$here"/0*.sh
do rc=0
   out=$("$check" "$@" 2>&1) || rc=$?
   printf '%s\n' "$out"
   if ! grep -qE '^(OK|FAULT|SKIP) ' <<< "$out"
   then printf 'ERROR %s: no verdict line (exit %s)\n' "${check##*/}" "$rc"
        error=$((error + 1))
   elif test "$rc" -eq 0
   then ok=$((ok + 1))
   elif test "$rc" -eq 1
   then fault=$((fault + 1))
   else skip=$((skip + 1))
   fi
done

printf '\nSummary: %s OK, %s FAULT, %s SKIP, %s ERROR\n' "$ok" "$fault" "$skip" "$error"
if test "$fault" -gt 0
then exit 1
fi
if test "$skip" -gt 0 || test "$error" -gt 0
then exit 2
fi
exit 0
