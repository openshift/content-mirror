build:
	go build -mod vendor ./cmd/content-mirror
.PHONY: build

build-image:
	docker build .
.PHONY: build-image

update-deps:
	GO111MODULE=on go mod vendor
.PHONY: update-deps
