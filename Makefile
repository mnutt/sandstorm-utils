GO ?= go
GOCACHE ?= /tmp/go-build
GOPROXY ?= off
GOSUMDB ?= off
PREFIX ?= /usr/local
DISTDIR ?= $(CURDIR)/dist
BINDIR ?= $(DISTDIR)/bin
RELEASEDIR ?= $(DISTDIR)/release
VERSION ?= dev
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
CMDS := get-public-id get-user-address close-session open-view post-activity get-session-request get-session-offer send-email stay-awake enter-grain

.PHONY: generate manifest build test install package clean

generate:
	./scripts/generate-capnp.sh

manifest:
	$(GO) run ./scripts/generate-utils-manifest.go

build:
	rm -rf $(BINDIR)
	mkdir -p $(BINDIR)
	for cmd in $(CMDS); do \
		GOCACHE=$(GOCACHE) GOPROXY=$(GOPROXY) GOSUMDB=$(GOSUMDB) $(GO) build -mod=mod -o $(BINDIR)/$$cmd ./cmd/$$cmd; \
	done

test:
	GOCACHE=$(GOCACHE) GOPROXY=$(GOPROXY) GOSUMDB=$(GOSUMDB) $(GO) test -mod=mod ./...

install: build
	mkdir -p $(PREFIX)/bin
	for cmd in $(CMDS); do \
		cp $(BINDIR)/$$cmd $(PREFIX)/bin/$$cmd; \
	done

package:
	rm -rf $(RELEASEDIR)/sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH)
	mkdir -p $(RELEASEDIR)/sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH)/bin
	for cmd in $(CMDS); do \
		GOOS=$(GOOS) GOARCH=$(GOARCH) GOCACHE=$(GOCACHE) GOPROXY=$(GOPROXY) GOSUMDB=$(GOSUMDB) $(GO) build -mod=mod -o $(RELEASEDIR)/sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH)/bin/$$cmd ./cmd/$$cmd; \
	done
	cp README.md $(RELEASEDIR)/sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH)/README.md
	tar -C $(RELEASEDIR) -czf $(RELEASEDIR)/sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH).tar.gz sandstorm-utils_$(VERSION)_$(GOOS)_$(GOARCH)

clean:
	rm -rf $(DISTDIR)
