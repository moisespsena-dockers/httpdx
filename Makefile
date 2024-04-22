DOCKER_CMD ?= docker
DOCKER_USERNAME ?= moisespsena
APPLICATION_NAME ?= httpdx
ADDR ?= ":10000"
SERVER_PORT ?= 80
GIT_HASH ?= $(shell git log --format="%h" -n 1)
tag := ${DOCKER_USERNAME}/${APPLICATION_NAME}

build:
	go build -ldflags='-X main.buildTime=$(shell date +%s)' -o dist/httpdx . && rm -rf /go/.gocache_docker

docker_build:
	$(DOCKER_CMD) build --build-arg PORT=$(SERVER_PORT) --tag ${tag}:${GIT_HASH} .

docker_run:
	$(DOCKER_CMD) run ${tag}

docker_push: docker_build
	$(DOCKER_CMD) push ${tag}:${GIT_HASH}

docker_release: docker_build
	docker pull ${tag}:${GIT_HASH}
	docker tag  ${tag}:${GIT_HASH} ${tag}:latest
	docker push ${tag}:latest
