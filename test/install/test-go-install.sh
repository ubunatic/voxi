#!/usr/bin/env bash
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

# The bind-mounted checkout belongs to the host user. GOFLAGS disables optional
# VCS stamping for both this command and the modifier build in voxi install.
go -C /src install ./cmd/voxi
test -x "$HOME/go/bin/voxi"
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

"$HOME/go/bin/voxi" install --modifierd
test -x "$HOME/system-root/usr/local/bin/voxi-modifierd"
test -s "$HOME/system-root/etc/systemd/system/voxi-modifierd.service"
grep -q -- 'enable --now voxi-modifierd.service' "$HOME/systemctl-calls"

# Make builds in a writable copy, then must use the same CLI installer.
mkdir -p "$HOME/voxi-source"
cp -R /src/audiolevel /src/cmd /src/internal /src/spec /src/systemd "$HOME/voxi-source/"
cp /src/*.go /src/go.mod /src/go.sum /src/Makefile "$HOME/voxi-source/"
make -C "$HOME/voxi-source" install
test -x "$HOME/.local/bin/voxi"
test "$(grep -c -- '--user enable --now voxi-agent.service' "$HOME/systemctl-calls")" -eq 3
test -s "$HOME/.local/share/man/man1/voxi.1"
grep -q '\.TH "VOXI"' "$HOME/.local/share/man/man1/voxi.1"
printf '%s\n' 'Go install, both voxi install modes, and make install passed'
