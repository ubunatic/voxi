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

install: ⚙️ build  # install voxi binary to ~/go/bin (user)
	go install ./cmd/voxi

install-debug: ⚙️ build-debug  # install debug binary to ~/go/bin
	go install -tags debug ./cmd/voxi

install-all: ⚙️ install  # install user binaries
	go install ./cmd/voxi-modifierd

restart-service: ⚙️ install  # rebuild, install, and restart the running voxi-agent user service
	systemctl --user restart voxi-agent.service

install-system: ⚙️ build  # install binary to PREFIX/bin via sudo (system-wide)
	sudo install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

install-modifierd: ⚙️ build-modifierd  # install voxi-modifierd and enable system service via sudo
	sudo install -m 0755 $(MODIFIER) $(PREFIX)/bin/$(MODIFIER)
	sudo cp systemd/voxi-modifierd.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable --now voxi-modifierd.service

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

canary-nested: ⚙️  # run nested GNOME Shell Wayland text-injection canary
	go run ./scripts/canary_nested

format: ⚙️  # format source code
	go fmt ./...

clean: ⚙️  # remove build artifacts
	rm -f $(BINARY) $(MODIFIER)

release: ⚙️  # release the project using uman
	@echo "To release this project using 'uman':"
	@echo "  1. Ensure you have 'uman', 'goreleaser', 'minisign', and 'fj' installed."
	@echo "  2. Run 'uman release' to release interactively."
