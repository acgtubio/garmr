PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
DOCDIR  ?= $(PREFIX)/share/doc/garmr
UNITDIR ?= /etc/systemd/system
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

UNIT := garmr@.service

.PHONY: build test install uninstall clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o garmr .

test:
	go vet ./...
	go test ./...

install: garmr
	install -Dm755 garmr $(DESTDIR)$(BINDIR)/garmr
	sed -e 's|@BINDIR@|$(BINDIR)|g' -e 's|@DOCDIR@|$(DOCDIR)|g' contrib/systemd/$(UNIT) \
		| install -Dm644 /dev/stdin $(DESTDIR)$(UNITDIR)/$(UNIT)
	install -Dm644 README.md $(DESTDIR)$(DOCDIR)/README.md
	@if [ -z "$(DESTDIR)" ]; then systemctl daemon-reload; fi
	@echo
	@echo "Installed. Enable the service for your user with:"
	@echo "  sudo systemctl enable --now garmr@$${SUDO_USER:-$$USER}"

uninstall:
	-for u in $$(systemctl list-units --all --plain --no-legend 'garmr@*' | awk '{print $$1}') \
	          $$(systemctl list-unit-files --plain --no-legend 'garmr@*' | awk '$$1 != "$(UNIT)" {print $$1}'); do \
		systemctl disable --now "$$u"; \
	done
	-$(BINDIR)/garmr clear
	rm -f $(BINDIR)/garmr $(UNITDIR)/$(UNIT)
	rm -rf $(DOCDIR)
	systemctl daemon-reload

garmr:
	@echo "garmr binary not found; run 'make build' (as your user) first" >&2; exit 1

clean:
	rm -f garmr
