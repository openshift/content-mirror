export CONTAINER_ENGINE ?= docker
export CONTAINER_ENGINE_OPTS ?= --platform linux/amd64

VOLUME_MOUNT_FLAGS = :z
ifeq ($(CONTAINER_ENGINE), docker)
	CONTAINER_USER=--user $(shell id -u):$(shell id -g)
else
	ifeq ($(shell uname -s), Darwin)
		# if you're running podman on macOS, don't set the SELinux label
		VOLUME_MOUNT_FLAGS =
	endif
	CONTAINER_USER=
endif

build:
	go build -mod=vendor ./cmd/content-mirror
.PHONY: build

build-image:
	$(CONTAINER_ENGINE) build -t content-mirror:latest .
.PHONY: build-image

debug:
	go build -gcflags="all=-N -l" -mod=vendor ./cmd/content-mirror
.PHONY: debug

debug-image:
	$(CONTAINER_ENGINE) build -t content-mirror:debug -f Dockerfile.dev .
.PHONY: debug-image

remote-debug: debug-image
	$(CONTAINER_ENGINE) run $(CONTAINER_ENGINE_OPTS) $(CONTAINER_USER) --rm -v "$(CONTENT_MIRROR_DATA)/key:/tmp/key$(VOLUME_MOUNT_FLAGS)" -v "$(CONTENT_MIRROR_DATA)/mirror-enterprise-basic-auth:/tmp/mirror-enterprise-basic-auth$(VOLUME_MOUNT_FLAGS)" -v "$(CONTENT_MIRROR_DATA)/repos:/tmp/repos$(VOLUME_MOUNT_FLAGS)" -p 8080:8080 -p 40000:40000 localhost/content-mirror:debug
.PHONY: remote-debug
