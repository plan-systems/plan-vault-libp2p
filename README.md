# plan-vault-libp2p

_An implementation of the PLAN storage layer using libp2p_

[![Build Status](https://github.com/plan-systems/plan-vault-libp2p/workflows/Build/badge.svg)](https://github.com/plan-systems/plan-vault-libp2p/actions)

### Development status

Early prototyping is underway. The only thing that's known to work is
that which is covered by tests. Not yet wired up for p2p. See `TODO`s
in the code for details; as the prototype solidifies this repo will
use GitHub issues to track progress.

### Build

Requires golang 1.16. To build: `make build`

### Development

* `make tools` will install tool chain dependencies (protoc, grpc).
* `make test` will run tests.
* `make check` will do linting and static analysis.

Continuous integration via [Github
actions](https://github.com/plan-systems/plan-vault-libp2p/actions)
will enforce that all commits pass `make test` and `make check`. The
repo also includes a self-describing Makefile:

```
$ make help

plan-vault-libp2p

build            build the vault binary
check            run linting and static analysis
clean            remove build artifacts
help             display this help message
nuke             remove build artifacts and all generated code
protos           generate code from protobufs
protosync        sync protobufs from the outer workspace
run              build and run the vault binary
test             run unit tests
tools            install protobuf tools
```
