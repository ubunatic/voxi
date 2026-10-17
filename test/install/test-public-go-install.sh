#!/usr/bin/env bash
# Test public go install workflow from vanity URL ubunatic.com/voxi/cmd/voxi
set -euo pipefail

export PATH="$HOME/bin:$HOME/go/bin:$HOME/.local/bin:$PATH"
export GOWORK=off
export GOBIN="$HOME/go/bin"
export GOFLAGS=-buildvcs=false
mkdir -p "$HOME/bin" "$GOBIN" "$HOME/system-root"

cat > "$HOME/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$HOME/systemctl-calls"
STUB
chmod +x "$HOME/bin/systemctl"

cat > "$HOME/bin/sudo" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if test "$1" = install
then mode="$3"
     source="$4"
     destination="$5"
     mkdir -p "$HOME/system-root$(dirname "$destination")"
     install -m "$mode" "$source" "$HOME/system-root$destination"
else "$@"
fi
STUB
chmod +x "$HOME/bin/sudo"

TARGET_VERSION="${VOXI_PUBLIC_VERSION:-latest}"
echo "==> Running go install ubunatic.com/voxi/cmd/voxi@${TARGET_VERSION}..."
go install "ubunatic.com/voxi/cmd/voxi@${TARGET_VERSION}"
test -x "$HOME/go/bin/voxi"

echo "==> Running voxi install..."
"$HOME/go/bin/voxi" install

test -x "$HOME/.local/bin/voxi"
test -x "$HOME/.local/bin/dotoold"
test -x "$HOME/.local/lib/voxi/crispasr/crispasr"
for unit in voxi-agent.service voxi-eager.service dotoold.service
do test -s "$HOME/.config/systemd/user/$unit"
done
grep -q -- '--user enable --now voxi-agent.service' "$HOME/systemctl-calls"
grep -q -- '--user enable --now dotoold.service' "$HOME/systemctl-calls"
test ! -e "$HOME/system-root/etc/systemd/system/voxi-modifierd.service"

echo "==> Running voxi install --modifierd..."
"$HOME/go/bin/voxi" install --modifierd
test -x "$HOME/system-root/usr/local/bin/voxi-modifierd"
test -s "$HOME/system-root/etc/systemd/system/voxi-modifierd.service"
grep -q -- 'enable --now voxi-modifierd.service' "$HOME/systemctl-calls"

printf '%s\n' "Public go install (${TARGET_VERSION}) and voxi install passed"
