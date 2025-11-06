.PHONY: install install-ubuntu image push test gen

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


test:
	# go clean -testcache
	go test ./test/...

###############################################################################
###                               Generation                                ###
###############################################################################
genImageName=indexer-price-oracle-code-generation:1.0.3
dockerGenRun=docker run --rm -v $(CURDIR):/workspace --workdir /workspace
genFiles=$(dockerGenRun) $(genImageName)
genFilesWithGoPath=$(dockerGenRun) -v $(GOPATH)/pkg:/go/pkg $(genImageName)

gen-docker-build:
	@if [ -z "$$(docker images -q $(genImageName))" ]; then \
		echo "Docker image $(genImageName) not found. Building..."; \
		docker build --build-arg GH_USER=$(GH_USER) \
			--build-arg GH_TOKEN=$(GH_TOKEN) \
			-t $(genImageName) -f gen/Dockerfile . ; \
	else \
		echo "Docker image $(genImageName) already exists. Skipping build."; \
	fi

gen-docker-rebuild:
	@docker image rm $(genImageName) || true
	make gen-docker-build

gen-goa:
	@$(genFiles) sh ./gen/gen-goa.sh

gen: gen-docker-build gen-goa