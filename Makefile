MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build

## display this help message
help:
	@echo -e "\033[32m"
	@echo "plan-vault-libp2p"
	@echo
	@awk '/^##.*$$/,/[a-zA-Z_-]+:/' $(MAKEFILE_LIST) | awk '!(NR%2){print $$0p}{p=$$0}' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}' | sort

# ----------------------------------------
# build

GOFILES = $(shell find . -type f -name '*.go')

.PHONY: build

## build the vault binary
build: bin/vault

bin/vault: $(GOFILES)
	GOPRIVATE='github.com/libp2p/*' go build -o bin/vault


# ----------------------------------------
# test

## run linting and static analysis
check:
	gofmt -w -s main.go

## build and run the vault binary
run: build
	./bin/vault

## run unit tests
test:
	go test -v ./... -count=1

# ----------------------------------------
# tooling

## install protobuf tools
tools:
	go get github.com/golang/protobuf/protoc-gen-go \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc

## remove build artifacts
clean:
	rm -rf ./bin

## remove build artifacts and all generated code
nuke: clean
	rm -f vault.pb.go
	rm -f vault_grpc.pb.go

# ----------------------------------------
# protobuffers

.PHONY: protos

## generate code from protobufs
protos: protos/vault.pb.go protos/vault_grpc.pb.go

protos/vault_grpc.pb.go: protos/vault.pb.go

protos/vault.pb.go: protos/vault.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		protos/vault.proto

.PHONY: protosync

# TODO: this is temporary until the proto definition settles down a
# bit, at which point we can git submodule the protobufs repo and
# use a proper go_package option
## sync protobufs from the outer workspace
protosync:
	sed 's~syntax = "proto3";~syntax = "proto3";\noption go_package = "github.com/plan-systems/plan-vault-libp2p/protos";~' ../plan-protobufs/pkg/vault/vault.proto > protos/vault.proto
