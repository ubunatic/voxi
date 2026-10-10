#!/usr/bin/env bash
# Check 2: the default sink and source (and the configured defaults) name nodes that exist.
# A default that names a vanished node leaves no device starred in wpctl status, and
# the desktop sound settings then hang. Read-only; the repair is printed, never run.
# Usage: 02-default-devices.sh [--pw-dump FILE] [--since TIME] (--since ignored here)
set -euo pipefail

CHECK=default-devices
source "$(dirname "$0")/lib.sh"
parse_args "$@"
need_cmd jq wpctl
load_pw_dump

node_names=$(jq -r '.[] | select(.type == "PipeWire:Interface:Node") | .info.props["node.name"] // empty' <<< "$PW_JSON") || verdict_skip "cannot read the node list"
defaults=$(jq -r '.[] | select(.type == "PipeWire:Interface:Metadata" and .props["metadata.name"] == "default") | .metadata[]? | select(.key | startswith("default.")) | "\(.key)\t\(.value.name? // "")"' <<< "$PW_JSON") || verdict_skip "cannot read the default metadata"
if test -z "$defaults"
then verdict_skip "no default.* metadata in the dump, is WirePlumber running?"
fi

# name_of KEY: prints the node name stored under a default.* metadata key
name_of() {
   awk -F '\t' -v key="$1" '$1 == key { print $2 }' <<< "$defaults"
}

# node_exists NAME
node_exists() {
   grep -Fxq -- "$1" <<< "$node_names"
}

# node_id NAME: prints the object ID of the node called NAME, empty if none
node_id() {
   jq -r --arg name "$1" 'first(.[] | select(.type == "PipeWire:Interface:Node" and .info.props["node.name"] == $name) | .id) // empty' <<< "$PW_JSON"
}

# first_node_of CLASS: prints the node name of the first node with media.class CLASS, empty if none
first_node_of() {
   jq -r --arg class "$1" 'first(.[] | select(.type == "PipeWire:Interface:Node" and .info.props["media.class"] == $class) | .info.props["node.name"]) // empty' <<< "$PW_JSON"
}

# pick_node CONFIGURED CLASS: the node to make default in place of a missing one: the configured
# node when it exists, else the first node of the class (empty if there is none)
pick_node() {
   if test -n "$1" && node_exists "$1"
   then printf '%s' "$1"
   else first_node_of "$2"
   fi
}

# active_fix KIND NODE: the repair for an active default that names no node. pw-metadata sets the
# active default, which applications read. Moving only the configured default does not fix it:
# live, that left the active default dangling.
active_fix() {
   if test -n "$2"
   then printf "pw-metadata 0 default.audio.%s '{\"name\":\"%s\"}' Spa:String:JSON" "$1" "$2"
   else printf 'wpctl status'
   fi
}

for kind in sink source
do class="Audio/${kind^}"
   name=$(name_of "default.audio.$kind")
   configured=$(name_of "default.configured.audio.$kind")
   fallback=$(pick_node "$configured" "$class")
   if test -z "$name"
   then add_problem "default.audio.$kind is empty (no default recorded)" "$(active_fix "$kind" "$fallback")"
   elif ! node_exists "$name"
   then add_problem "default.audio.$kind names '$name', which no node has" "$(active_fix "$kind" "$fallback")"
   fi
   if test -n "$configured" && ! node_exists "$configured"
   then id=$(node_id "$fallback")
        if test -n "$id"
        then repair="wpctl set-default $id"
             add_problem "default.configured.audio.$kind names '$configured', which is gone" "$repair"
        else add_problem "default.configured.audio.$kind names '$configured', which is gone, and no $kind is present" "wpctl status"
        fi
   fi
done

if test -n "$problems"
then verdict_fault "$repairs" "$problems"
fi
verdict_ok "default sink '$(name_of default.audio.sink)' and source '$(name_of default.audio.source)' both exist"
