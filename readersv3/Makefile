APP ?= generic-test-reader
TARGET ?= $(shell GO111MODULE=on go env GOOS)-$(shell GO111MODULE=on go env GOARCH)
VERSION ?=

GO_CACHE_ENV = GO111MODULE=on GOCACHE=/tmp/wisemed-releasectl-gocache GOMODCACHE=/tmp/wisemed-releasectl-gomodcache
RELEASECTL = $(GO_CACHE_ENV) go run ./tools/releasectl
VERSION_ARG = $(if $(strip $(VERSION)),--version $(VERSION),)

.PHONY: build installer update release build-all package-all

build:
	$(RELEASECTL) build --app $(APP) --target $(TARGET) $(VERSION_ARG)

installer:
	$(RELEASECTL) package --app $(APP) --target $(TARGET) $(VERSION_ARG)

update:
	$(RELEASECTL) package-update --app $(APP) --target $(TARGET) $(VERSION_ARG)

release:
	$(RELEASECTL) release --app $(APP) --target $(TARGET) $(VERSION_ARG)

build-all:
	$(RELEASECTL) build-all $(VERSION_ARG)

package-all:
	$(RELEASECTL) package-all $(VERSION_ARG)
