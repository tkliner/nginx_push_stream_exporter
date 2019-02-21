GO           ?= go
GOFMT        ?= $(GO)fmt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
GOOPTS       ?=
GOHOSTOS     ?= $(shell $(GO) env GOHOSTOS)
GOHOSTARCH   ?= $(shell $(GO) env GOHOSTARCH)

GO_VERSION        ?= $(shell $(GO) version)
GO_VERSION_NUMBER ?= $(word 3, $(GO_VERSION))
PRE_GO_111        ?= $(shell echo $(GO_VERSION_NUMBER) | grep -E 'go1\.(10|[0-9])\.')

unexport GOVENDOR
ifeq (, $(PRE_GO_111))
	ifneq (,$(wildcard go.mod))
		# Enforce Go modules support just in case the directory is inside GOPATH (and for Travis CI).
		GO111MODULE := on

		ifneq (,$(wildcard vendor))
			# Always use the local vendor/ directory to satisfy the dependencies.
			GOOPTS := $(GOOPTS) -mod=vendor
		endif
	endif
else
	ifneq (,$(wildcard go.mod))
		ifneq (,$(wildcard vendor))
$(warning This repository requires Go >= 1.11 because of Go modules)
$(warning Some recipes may not work as expected as the current Go runtime is '$(GO_VERSION_NUMBER)')
		endif
	else
		# This repository isn't using Go modules (yet).
		GOVENDOR := $(FIRST_GOPATH)/bin/govendor
	endif

	unexport GO111MODULE
endif
PROMU        := $(FIRST_GOPATH)/bin/promu
STATICCHECK  := $(FIRST_GOPATH)/bin/staticcheck
pkgs          = ./...

ifeq (arm, $(GOHOSTARCH))
	GOHOSTARM ?= $(shell GOARM= $(GO) env GOARM)
	GO_BUILD_PLATFORM ?= $(GOHOSTOS)-$(GOHOSTARCH)v$(GOHOSTARM)
else
	GO_BUILD_PLATFORM ?= $(GOHOSTOS)-$(GOHOSTARCH)
endif

PROMU_VERSION ?= 0.2.0
PROMU_URL     := https://github.com/prometheus/promu/releases/download/v$(PROMU_VERSION)/promu-$(PROMU_VERSION).$(GO_BUILD_PLATFORM).tar.gz
STATICCHECK_VERSION ?= 2019.1
STATICCHECK_URL     := https://github.com/dominikh/go-tools/releases/download/$(STATICCHECK_VERSION)/staticcheck_$(GOHOSTOS)_$(GOHOSTARCH)

PREFIX                  ?= $(shell pwd)
BIN_DIR                 ?= $(shell pwd)
DOCKER_IMAGE_TAG        ?= $(subst /,-,$(shell git rev-parse --abbrev-ref HEAD))
DOCKER_REPO             ?= prom

ifeq ($(GOHOSTARCH),amd64)
        ifeq ($(GOHOSTOS),$(filter $(GOHOSTOS),linux freebsd darwin windows))
                # Only supported on amd64
                test-flags := -race
        endif
endif

.PHONY: all
all: precheck style staticcheck unused build test

# This rule is used to forward a target like "build" to "common-build".  This
# allows a new "build" target to be defined in a Makefile which includes this
# one and override "common-build" without override warnings.
%: common-% ;

.PHONY: common-build
common-build: promu
	@echo ">> building binaries"
	GO111MODULE=$(GO111MODULE) $(PROMU) build --prefix $(PREFIX)

.PHONY: common-tarball
common-tarball: promu
	@echo ">> building release tarball"
	$(PROMU) tarball --prefix $(PREFIX) $(BIN_DIR)

.PHONY: common-docker
common-docker:
	docker build -t "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" .

.PHONY: common-docker-publish
common-docker-publish:
	docker push "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)"

.PHONY: common-docker-tag-latest
common-docker-tag-latest:
	docker tag "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)" "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME):latest"

.PHONY: promu
promu: $(PROMU)

$(PROMU):
	$(eval PROMU_TMP := $(shell mktemp -d))
	curl -s -L $(PROMU_URL) | tar -xvzf - -C $(PROMU_TMP)
	mkdir -p $(FIRST_GOPATH)/bin
	cp $(PROMU_TMP)/promu-$(PROMU_VERSION).$(GO_BUILD_PLATFORM)/promu $(FIRST_GOPATH)/bin/promu
	rm -r $(PROMU_TMP)

$(STATICCHECK):
	mkdir -p $(FIRST_GOPATH)/bin
	curl -s -L $(STATICCHECK_URL) > $(STATICCHECK)

ifdef GOVENDOR
.PHONY: $(GOVENDOR)
$(GOVENDOR):
	GOOS= GOARCH= $(GO) get -u github.com/kardianos/govendor
endif
