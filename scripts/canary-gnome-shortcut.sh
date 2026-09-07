#!/bin/sh
set -eu

schema=org.gnome.settings-daemon.plugins.media-keys
printf 'Desktop: %s\n' "${XDG_CURRENT_DESKTOP:-unset}"
command -v gsettings
printf 'Custom shortcut paths (read-only):\n'
gsettings get "$schema" custom-keybindings
printf 'Existing Super+X assignments (read-only):\n'
found=0
for candidate in org.gnome.desktop.wm.keybindings "$schema"; do
	if gsettings list-recursively "$candidate" | grep -Fi "'<Super>x'"; then found=1; fi
done
if [ "$found" -eq 0 ]; then printf 'none in built-in GNOME shortcut schemas\n'; fi

# Prove the relocatable schema write/read mechanism in an isolated keyfile backend.
# This never connects to or changes the user's live dconf database.
canary_dir=$(mktemp -d)
trap 'rm -rf "$canary_dir"' EXIT
export GSETTINGS_BACKEND=keyfile
export XDG_CONFIG_HOME="$canary_dir"
path=/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/voxi-canary/
child=org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:$path
gsettings set "$child" name 'Voxi Canary'
gsettings set "$child" command '/tmp/voxi-canary record toggle'
gsettings set "$child" binding '<Super>x'
gsettings set "$schema" custom-keybindings "['$path']"
printf 'Isolated write/read: %s | %s | %s\n' \
	"$(gsettings get "$child" name)" \
	"$(gsettings get "$child" command)" \
	"$(gsettings get "$child" binding)"
printf 'Live settings were only read; isolated writes are discarded now.\n'
