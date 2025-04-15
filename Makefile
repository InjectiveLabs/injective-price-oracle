APP_VERSION = $(shell git describe --abbrev=0 --tags)
GIT_COMMIT = $(shell git rev-parse --short HEAD)
BUILD_DATE = $(shell date -u "+%Y%m%d-%H%M")
VERSION_PKG = github.com/InjectiveLabs/injective-price-oracle/version
VERSION_FLAGS = "-X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)"
export GOPROXY = direct
export VERSION_FLAGS

all:

image:
	docker build --build-arg GIT_COMMIT=$(GIT_COMMIT) -t $(IMAGE_NAME):local -f Dockerfile .
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):$(GIT_COMMIT)
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):latest

push:
	docker push $(IMAGE_NAME):$(GIT_COMMIT)
	docker push $(IMAGE_NAME):latest

install:
	go install \
		-tags muslc \
		-ldflags $(VERSION_FLAGS) \
		./cmd/...

install-ubuntu:
	go install \
		-ldflags $(VERSION_FLAGS) \
		./cmd/...

.PHONY: install image push test gen

test:
	# go clean -testcache
	go test ./test/...
