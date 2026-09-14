.PHONY: ⚙️ 🤖  # ⚙️ = manual/once, 🤖 = managed

_prim := \033[36m
_rst  := \033[0m

BINARY   ?= voxi
MODIFIER ?= voxi-modifierd
PREFIX   ?= /usr/local

help: 🤖  # show this help
	@grep -E '^[a-zA-Z_-]+:.*[⚙🤖].*#+' $(MAKEFILE_LIST) | \
	awk 'BEGIN {FS = ":.*#+ "}; {printf "    $(_prim)%-15s$(_rst) %s\n", $$1, $$2}'

preflight: ⚙️  # check toolchains and dependencies
	@command -v go >/dev/null || (echo "❌ go is not installed" && exit 1)

build: ⚙️  # build the main voxi CLI binary
	go build -o $(BINARY) ./cmd/voxi

build-debug: ⚙️  # build binary with debug/canary commands
	go build -tags debug -o $(BINARY) ./cmd/voxi

build-modifierd: ⚙️  # build the physical modifier daemon binary
	go build -o $(MODIFIER) ./cmd/voxi-modifierd

build-all: ⚙️ build build-modifierd  # build all binaries

run: ⚙️ build  # run voxi monitor
	./$(BINARY) monitor

install: ⚙️ build  # install the binary, dependencies, and user services through voxi install
	./$(BINARY) install

install-debug: ⚙️ build-debug  # install debug binary to ~/.local/bin and symlink to ~/go/bin
	@mkdir -p $(HOME)/.local/bin $(HOME)/go/bin
	install -m 0755 $(BINARY) $(HOME)/.local/bin/$(BINARY)
	ln -sf $(HOME)/.local/bin/$(BINARY) $(HOME)/go/bin/$(BINARY)

install-all: ⚙️ install  # compatibility alias for the converged user install

install-dotool: ⚙️  # install dotool (direct keystroke injection) to ~/go/bin
	go install git.sr.ht/~geb/dotool@latest

# DOTOOL_XKB_LAYOUT: auto-detected from localectl's X11 Layout (this machine's actual
# physical keyboard layout); override per machine, e.g. `make DOTOOL_XKB_LAYOUT=us install-dotoold`
DOTOOL_XKB_LAYOUT ?= $(shell localectl status 2>/dev/null | awk -F': *' '/X11 Layout/{print $$2}')

install-dotoold: ⚙️ install-dotool  # install dotoold/dotoolc scripts + dotoold.service, enable+start it (persistent uinput daemon needed for reliable GNOME Wayland typing; issue 081)
	install -m 0755 "$$(go list -m -f '{{.Dir}}' git.sr.ht/~geb/dotool@latest)/dotoold" "$$(go list -m -f '{{.Dir}}' git.sr.ht/~geb/dotool@latest)/dotoolc" $(HOME)/go/bin/
	mkdir -p $(HOME)/.config/systemd/user
	sed 's|@DOTOOL_XKB_LAYOUT@|$(DOTOOL_XKB_LAYOUT)|' systemd/dotoold.service > $(HOME)/.config/systemd/user/dotoold.service
	systemctl --user daemon-reload
	systemctl --user enable --now dotoold.service

restart-service: ⚙️ install  # rebuild, install, and restart the running voxi-agent user service
	systemctl --user restart voxi-agent.service

install-system: ⚙️ build  # install binary to PREFIX/bin via sudo (system-wide)
	sudo install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

install-crispasr: ⚙️  # download official CrispASR release binary (crispasr, needed by the cohere-transcribe engine; issue 078)
	@arch="$$(uname -m)"; \
	case "$$arch" in \
		x86_64) asset=crispasr-linux-x86_64.tar.gz ;; \
		aarch64|arm64) asset=crispasr-linux-arm64.tar.gz ;; \
		*) echo "❌ install-crispasr: unsupported architecture $$arch (CrispASR has no prebuilt release for it)"; exit 1 ;; \
	esac; \
	dir="$(HOME)/.local/lib/voxi/crispasr"; \
	cache="$(HOME)/.cache/voxi/$$asset"; \
	mkdir -p "$$dir" "$(HOME)/go/bin" "$$(dirname "$$cache")"; \
	if [ ! -s "$$cache" ] || ! tar -tzf "$$cache" >/dev/null 2>&1; then \
		echo "Downloading $$asset from CrispStrobe/CrispASR latest release..."; \
		curl -fL -o "$$cache" "https://github.com/CrispStrobe/CrispASR/releases/latest/download/$$asset" || \
			(echo "❌ install-crispasr: download failed. Place $$asset into $$cache manually or run 'voxi install'."; exit 1); \
	fi; \
	tar -xzf "$$cache" -C "$$dir" --strip-components=1; \
	ln -sf "$$dir/crispasr" "$(HOME)/go/bin/crispasr"; \
	"$(HOME)/go/bin/crispasr" --version

install-modifierd: ⚙️ build  # install user services and optional system modifier daemon
	./$(BINARY) install --modifierd

install-user-services: ⚙️  # install systemd user service units
	mkdir -p $(HOME)/.config/systemd/user
	cp systemd/voxi-agent.service systemd/voxi-eager.service $(HOME)/.config/systemd/user/
	systemctl --user daemon-reload

uninstall: ⚙️  # remove installed binaries and services
	rm -f $(HOME)/go/bin/$(BINARY) $(HOME)/go/bin/$(MODIFIER)
	sudo rm -f $(PREFIX)/bin/$(BINARY) $(PREFIX)/bin/$(MODIFIER) /etc/systemd/system/voxi-modifierd.service
	sudo systemctl daemon-reload || true

test: ⚙️  # run tests and vet
	go vet ./...
	go test ./...

check: ⚙️ test validate-spec  # run tests, vet, and spec validation

validate-spec: ⚙️  # validate spec/*.yaml against its embedded schema and loader
	go test ./spec/...

test-debug: ⚙️  # run tests with debug tag
	go vet -tags debug ./...
	go test -tags debug ./...

test-install-podman: ⚙️  # test Go and curl installation workflows in clean containers
	./scripts/test-install-podman.sh

test-install: ⚙️ test-install-podman  # alias for test-install-podman

test-e2e: ⚙️  # run headless end-to-end integration test in container via podman
	./scripts/test-e2e-headless.sh

canary-nested: ⚙️  # run nested GNOME Shell Wayland text-injection canary
	go run ./scripts/canary_nested

format: ⚙️  # format source code
	go fmt ./...

clean: ⚙️  # remove build artifacts
	rm -f $(BINARY) $(MODIFIER)

release: check ⚙️  # release the project using harnez
	harnez release
