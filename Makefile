build:
	go build ./cmd/content-mirror
.PHONY: build

build-image:
	podman build .
.PHONY: build-image

update-deps:
	glide update -v --skip-test
.PHONY: update-deps
