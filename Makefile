PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build install test serve

build:
	go build -o bin/meetbarctl ./cmd/meetbarctl
	go build -o bin/meetbard ./cmd/meetbard

install: build
	install -Dm755 bin/meetbarctl $(BINDIR)/meetbarctl
	install -Dm755 bin/meetbard $(BINDIR)/meetbard
	install -Dm644 systemd/meetbard.service $(HOME)/.config/systemd/user/meetbard.service
	systemctl --user daemon-reload

test:
	go test ./...

serve: build
	./bin/meetbarctl serve
