.PHONY: default install build test lint clean

BINARY ?= k8s-replicator

GOCMD = go
GOLINTCMD = golint
GOFLAGS ?= $(GOFLAGS:)
LDFLAGS =
RUN ?= "."

default: build

install:
	"$(GOCMD)" mod download

build:
	"$(GOCMD)" build ${GOFLAGS} ${LDFLAGS} -o "${BINARY}"

test:
	"$(GOCMD)" test -timeout 1800s -v ./... -run "${RUN}"

lint:
	"$(GOLINTCMD)" ./...

clean:
	"$(GOCMD)" clean -i
