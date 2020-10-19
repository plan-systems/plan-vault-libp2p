MAKEFLAGS += --warn-undefined-variables
SHELL = /bin/bash -o nounset -o errexit -o pipefail
.DEFAULT_GOAL = build

# ----------------------------------------
# build

.PHONY: build
build: bin/vault

bin/vault:
	go build -o bin/vault


# ----------------------------------------
# test

check:
	gofmt -w -s main.go

test:
	go test -v .

# ----------------------------------------
# tooling

tools:
	go get github.com/golang/protobuf/protoc-gen-go \
		google.golang.org/grpc/cmd/protoc-gen-go-grpc

clean:
	rm -rf ./bin

nuke: clean
	rm -f vault.pb.go
	rm -f vault_grpc.pb.go

# ----------------------------------------
# protobuffers

.PHONY: protos
protos: protos/vault.pb.go protos/vault_grpc.pb.go

protos/vault_grpc.pb.go: protos/vault.pb.go

protos/vault.pb.go: protos/vault.proto
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		protos/vault.proto

# TODO: this is temporary until the proto definition settles down a
# bit, at which point we can git submodule the protobufs repo and
# use a proper go_package option
.PHONY: protosync
protosync:
	sed 's~syntax = "proto3";~syntax = "proto3";\noption go_package = "github.com/plan-systems/plan-vault-libp2p/protos";~' ../plan-protobufs/pkg/vault/vault.proto > protos/vault.proto
