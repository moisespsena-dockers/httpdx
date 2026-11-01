DOCKER_CMD ?= docker
STATICCHECK_VERSION ?= 2026.2.2
DOCKER_USERNAME ?= moisespsena
APPLICATION_NAME ?= httpdx
ADDR ?= ":10000"
SERVER_PORT ?= 80
GIT_HASH ?= $(shell git log --date=format:'%Y%m%d%H%M%S' --format="%cd-%h" -n 1)
tag := ${DOCKER_USERNAME}/${APPLICATION_NAME}
cwd := $(realpath $(shell pwd))
go_pkg_dir := $(realpath ${cwd}/../../../../pkg)

build:
	go build -ldflags='-X main.buildTime=$(shell date +%s)' -o dist/httpdx . && rm -rf /go/.gocache_docker

docker_build_binary:
	$(DOCKER_CMD) run \
		--user $(shell id -u):$(shell id -g) \
		-v ${cwd}:/src \
		-v ${go_pkg_dir}:/go/pkg \
		-i \
		-e GOCACHE=/tmp/.gocache \
		golang:1.22-bullseye \
		bash -c 'cd /src && go build -o /src/dist/httpdx_go1.22_bullseye'

docker_build:
	$(DOCKER_CMD) build \
	--build-arg PORT=$(SERVER_PORT) \
	--tag ${tag}:${GIT_HASH} .
	$(DOCKER_CMD) tag  ${tag}:${GIT_HASH} ${tag}:latest

docker_run:
	$(DOCKER_CMD) run -p ${ADDR}:${SERVER_PORT} ${tag}:${GIT_HASH}


docker_build_all: docker_build_binary docker_build
docker_build_run: docker_build docker_run
docker_build_all_run: docker_build_all docker_run

docker_push: docker_build_all
	$(DOCKER_CMD) push ${tag}:${GIT_HASH}

docker_release: docker_push
	$(DOCKER_CMD) pull ${tag}:${GIT_HASH}
	$(DOCKER_CMD) push ${tag}:latest



# The compose tests (test/compose): servers and clients in containers, behind a
# front that drops idle connections, and a check of what survives. The
# binaries are built here (no package installed in the image).
test-compose:
	CGO_ENABLED=0 go build -o test/compose/bin/httpdx .
	CGO_ENABLED=0 go build -o test/compose/bin/ ./internal/testtools/...
	$(DOCKER_CMD) compose -f test/compose/compose.yml up --build --abort-on-container-exit --exit-code-from check; \
	code=$$?; $(DOCKER_CMD) compose -f test/compose/compose.yml down --remove-orphans >/dev/null 2>&1; exit $$code

.PHONY: test-compose

# lint: the code formatted (gofmt), go vet and staticcheck (STATICCHECK_VERSION,
# run by go run: nothing to install).
lint:
	@unformatted=$$(gofmt -l .); if [ -n "$$unformatted" ]; then \
		echo "not formatted (gofmt -w):"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

# test: the client and the server in process.
test:
	go test -race -count=1 ./...

# verify: what a commit must pass — lint, the tests in process and in
# containers (test-compose). The verify workflow runs it.
verify: lint test test-compose

.PHONY: lint test verify
