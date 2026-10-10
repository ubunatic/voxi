#!/usr/bin/env bash
# Shared helpers for the audio doctor checks. Sourced by the NN-*.sh checks, never run directly.
# Each check prints one "OK|FAULT|SKIP <check>: <explanation>" line and exits
# 0 (ok), 1 (fault found) or 2 (cannot check). Nothing here changes system state.

problems=""
repairs=""

# parse_args sets PW_DUMP_FILE (--pw-dump FILE), SINCE (--since TIME), RUN_PROBE (--no-probe
# turns it off) and PW_LINK_FILE (--pw-link FILE, canned pw-link -l output for check 6).
# Every check accepts all of them; the ones that do not need a value ignore it.
parse_args() {
   PW_DUMP_FILE=""
   SINCE=""
   RUN_PROBE=yes
   PW_LINK_FILE=""
   while test $# -gt 0
   do if test "$1" = "--pw-dump" && test $# -ge 2
      then PW_DUMP_FILE="$2"
           shift
      elif test "$1" = "--since" && test $# -ge 2
      then SINCE="$2"
           shift
      elif test "$1" = "--pw-link" && test $# -ge 2
      then PW_LINK_FILE="$2"
           shift
      elif test "$1" = "--no-probe"
      then RUN_PROBE=no
      else printf 'usage: %s [--pw-dump FILE] [--since TIME] [--no-probe] [--pw-link FILE]\n' "${0##*/}" >&2
           exit 2
      fi
      shift
   done
}

# verdict_ok EXPLANATION
verdict_ok() {
   printf 'OK %s: %s\n' "$CHECK" "$*"
   exit 0
}

# verdict_fault REPAIR EXPLANATION
verdict_fault() {
   local repair="$1"
   shift
   printf 'FAULT %s: %s\n' "$CHECK" "$*"
   printf '  repair (printed, not run): %s\n' "$repair"
   exit 1
}

# verdict_skip EXPLANATION
verdict_skip() {
   printf 'SKIP %s: %s\n' "$CHECK" "$*"
   exit 2
}

# need_cmd TOOL...: SKIP when a tool the check depends on is not installed
need_cmd() {
   local cmd
   for cmd in "$@"
   do if ! command -v "$cmd" >/dev/null 2>&1
      then verdict_skip "$cmd is not installed, cannot check"
      fi
   done
}

# add_problem EXPLANATION REPAIR: collects a problem and its repair for verdict_fault
add_problem() {
   problems="${problems:+$problems, }$1"
   repairs="${repairs:+$repairs && }$2"
}

# load_pw_dump: sets PW_JSON from --pw-dump FILE, or from a live pw-dump
load_pw_dump() {
   if test -n "$PW_DUMP_FILE"
   then if ! test -r "$PW_DUMP_FILE"
        then verdict_skip "cannot read $PW_DUMP_FILE"
        fi
        PW_JSON=$(cat "$PW_DUMP_FILE")
   else need_cmd pw-dump
        PW_JSON=$(pw-dump 2>/dev/null) || verdict_skip "pw-dump failed, PipeWire is not reachable"
   fi
   if ! printf '%s\n' "$PW_JSON" | jq -e 'type == "array"' >/dev/null 2>&1
   then verdict_skip "no pw-dump JSON array from ${PW_DUMP_FILE:-live PipeWire}"
   fi
}
