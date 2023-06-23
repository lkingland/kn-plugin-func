# ##
#
# Run 'make help' for a summary
#
# ##

# Binaries
BIN         := func
BIN_DARWIN_AMD64   ?= $(BIN)_darwin_amd64
BIN_DARWIN_ARM64   ?= $(BIN)_darwin_arm64
BIN_LINUX_AMD64   ?= $(BIN)_linux_amd64
BIN_LINUX_ARM64   ?= $(BIN)_linux_arm64
BIN_LINUX_PPC64LE ?= $(BIN)_linux_ppc64le
BIN_LINUX_S390X   ?= $(BIN)_linux_s390x
BIN_WINDOWS ?= $(BIN)_windows_amd64.exe

# Version
# A verbose version is built into the binary including a date stamp, git commit
# hash and the version tag of the current commit (semver) if it exists.
# If the current commit does not have a semver tag, 'tip' is used, unless there
# is a TAG environment variable. Precedence is git tag, environment variable, 'tip'
HASH    := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG    := $(shell git tag --points-at HEAD | head -1)
VTAG    := $(shell [ -z $(VTAG) ] && echo $(ETAG) || echo $(VTAG))
VERS    ?= $(shell git describe --tags --match 'v*')
KVER    ?= $(shell git describe --tags --match 'knative-*')
LDFLAGS := "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.kver=$(KVER) -X main.hash=$(HASH)"

# All Code prerequisites, including generated files, etc.
CODE := $(shell find . -name '*.go') \
				generate/zz_filesystem_generated.go go.mod \
				schema/func_yaml-schema.json \
				templates/certs/ca-certificates.crt

# Default Target
all: check test build docs

# Help Text
# Headings: lines with `##$` comment prefix
# Targets:  printed if their line includes a `##` comment
.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'The default when running "make" without specifying a target is a'
	@echo 'full build, including linting, testing, and generating metadata such'
	@echo 'as docs and schema (repeated invocations will attempt to cache).'
	@echo ''
	@echo 'For a completely clean build, which also regenerates assets ignoring'
	@echo 'cache, run "make clean && make".  This command should also be run'
	@echo 'prior to submitting any pull requests, with any generated files'
	@echo 'or changes included in the PR.'
	@echo ''
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


###############
##@ Development
###############

.PHONY: build
build: $(BIN) ## Build binary for current OS

$(BIN): $(CODE)
	# Building
	env CGO_ENABLED=0 go build -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: tst tst-checksum
tst: tst-checksum .tst_stamp
.tst_stamp: .tst_checksum
	# Testing
	go test -race -cover -coverprofile=coverage.txt ./...
	touch .tst_stamp
tst-checksum:
	@go run generate/checkstamp/main.go .tst_checksum ./...


.PHONY: test
test: .test_stamp ## Run tests
.test_stamp: $(CODE)
	# Testing
	go test -race -cover -coverprofile=coverage.txt ./...
	@touch .test_stamp

.PHONY: check
check: .check_stamp ## Check code quality (lint)
# stamp allows caching.  Run `make clean` to force receheck.
.check_stamp: $(CODE) bin/golangci-lint
	# Checking code quality with linter
	./bin/golangci-lint run --timeout 600s
	cd test && ../bin/golangci-lint run --timeout 600s
	@touch .check_stamp

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.53.2

.PHONY: clean
clean: ## Remove generated files. Use 'make clean && make' for a fully "clean" build.
	# Cleaning up binaries, generated files and stamp sentinel files
	rm -f $(BIN) $(BIN_WINDOWS) $(BIN_LINUX) $(BIN_DARWIN_AMD64) $(BIN_DARWIN_ARM64)
	rm -f generate/zz_filesystem_generated.go
	rm -f templates/certs/ca-certificates.crt
	rm -f schema/func_yaml-schema.json
	rm -f coverage.txt
	rm -f .docs_stamp
	rm -f .check_stamp
	rm -f .test_stamp
	rm -f .templates_stamp

.PHONY: docs
docs: .docs_stamp
.docs_stamp: $(CODE)
	# Generating command reference docs
	go run docs/generator/main.go
	@touch .docs_stamp

#############
##@ Templates
#############

.PHONY: templates
templates: .templates_checkstamp generate/zz_filesystem_generated.go ## Build the embedded templates

generate/zz_filesystem_generated.go: .templates_stamp templates/certs/ca-certificates.crt
	@rm -f templates/go/cloudevents/go.sum
	@rm -f templates/go/http/go.sum
	@rm -rf templates/node/cloudevents/node_modules
	@rm -rf templates/node/http/node_modules
	@rm -rf templates/python/cloudevents/__pycache__
	@rm -rf templates/python/http/__pycache__
	@rm -rf templates/typescript/cloudevents/node_modules
	@rm -rf templates/typescript/http/node_modules
	@rm -rf templates/typescript/cloudevents/build
	@rm -rf templates/typescript/http/build
	@rm -rf templates/rust/cloudevents/target
	@rm -rf templates/rust/http/target
	@rm -rf templates/quarkus/cloudevents/target
	@rm -rf templates/quarkus/http/target
	@rm -rf templates/springboot/cloudevents/target
	@rm -rf templates/springboot/http/target
	@rm -f templates/**/.DS_Store
	# Generating embedded templates filesystem
	go generate pkg/functions/templates_embedded.go

# templates_checkstamp creates or updates .templates_stamp with the hash of
# the files in ./templates, allowing rebuild on file deletions.
.PHONY: .templates_checkstamp
.templates_checkstamp:
	@go run generate/checkstamp/main.go .templates_stamp ./templates

# Checks that the generated embedded filesystem has been modified.
# In CI this is combined with a `make clean` to check that the templates
# embedded fileystem is up-to-date.
.PHONY: templates-modified-check
templates-modified-check: generate/zz_filesystem_generated.go
	@if [[ -n $$(git diff -- schema/func_yaml-schema.json) ]]; \
		then echo "\nEmbedded template filesystem (pkg/functions/templates_embedded.go) has changes, please run 'make templates' and commit the result.\n" >&2; \
	exit 1; fi

# TODO: add linters for other templates
check-templates: check-rust

check-rust: ## Lint Rust templates
	cd templates/rust/cloudevents && cargo clippy && cargo clean
	cd templates/rust/http && cargo clippy && cargo clean

.PHONY: test-templates
test-templates: test-go test-node test-python test-quarkus test-springboot test-rust test-typescript ## Run all template tests

.PHONY: test-go
test-go: ## Test Go templates
	cd templates/go/cloudevents && go mod tidy && go test
	cd templates/go/http && go mod tidy && go test

.PHONY: test-node
test-node: ## Test Node templates
	cd templates/node/cloudevents && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

.PHONY: test-python
test-python: ## Test Python templates
	cd templates/python/cloudevents && pip3 install -r requirements.txt && python3 test_func.py && rm -rf __pycache__
	cd templates/python/http && python3 test_func.py && rm -rf __pycache__

.PHONY: test-quarkus
test-quarkus: ## Test Quarkus templates
	cd templates/quarkus/cloudevents && mvn test && mvn clean
	cd templates/quarkus/http && mvn test && mvn clean

.PHONY: test-springboot
test-springboot: ## Test Spring Boot templates
	cd templates/springboot/cloudevents && mvn test && mvn clean
	cd templates/springboot/http && mvn test && mvn clean

.PHONY: test-rust
test-rust: ## Test Rust templates
	cd templates/rust/cloudevents && cargo test && cargo clean
	cd templates/rust/http && cargo test && cargo clean

.PHONY: test-typescript
test-typescript: ## Test Typescript templates
	cd templates/typescript/cloudevents && npm ci && npm test && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npm test && rm -rf node_modules build

###################
##@ Extended Testing (cluster required)
###################

.PHONY: test-integration
test-integration: ## Run integration tests using an available cluster.
	go test -tags integration -timeout 30m --coverprofile=coverage.txt ./... -v

.PHONY: func-instrumented
func-instrumented: ## Func binary that is instrumented for e2e tests
	env CGO_ENABLED=1 go build -ldflags $(LDFLAGS) -cover -o func ./cmd/func

.PHONY: test-e2e
test-e2e: func-instrumented ## Run end-to-end tests using an available cluster.
	./test/e2e_extended_tests.sh

.PHONY: test-e2e-runtime
test-e2e-runtime: func-instrumented ## Run end-to-end lifecycle tests using an available cluster for a single runtime.
	./test/e2e_lifecycle_tests.sh $(runtime)

.PHONY: test-e2e-on-cluster
test-e2e-on-cluster: func-instrumented ## Run end-to-end on-cluster build tests using an available cluster.
	./test/e2e_oncluster_tests.sh

######################
##@ Release Artifacts
######################

.PHONY: cross-platform
cross-platform: darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 linux-ppc64le linux-s390x windows ## Build all distributable (cross-platform) binaries

.PHONY: darwin-arm64
darwin-arm64: $(BIN_DARWIN_ARM64) ## Build for mac M1

$(BIN_DARWIN_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DARWIN_ARM64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: darwin-amd64
darwin-amd64: $(BIN_DARWIN_AMD64) ## Build for Darwin (macOS)

$(BIN_DARWIN_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DARWIN_AMD64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: linux-amd64
linux-amd64: $(BIN_LINUX_AMD64) ## Build for Linux amd64

$(BIN_LINUX_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_LINUX_AMD64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: linux-arm64
linux-arm64: $(BIN_LINUX_ARM64) ## Build for Linux arm64

$(BIN_LINUX_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BIN_LINUX_ARM64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: linux-ppc64le
linux-ppc64le: $(BIN_LINUX_PPC64LE) ## Build for Linux ppc64le

$(BIN_LINUX_PPC64LE): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -o $(BIN_LINUX_PPC64LE) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: linux-s390x
linux-s390x: $(BIN_LINUX_S390X) ## Build for Linux s390x

$(BIN_LINUX_S390X): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -o $(BIN_LINUX_S390X) -ldflags $(LDFLAGS) ./cmd/$(BIN)

.PHONY: windows
windows: $(BIN_WINDOWS) ## Build for Windows

$(BIN_WINDOWS): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_WINDOWS) -ldflags $(LDFLAGS) ./cmd/$(BIN)

######################
##@ Other
######################

.PHONY: schema
schema: schema-clean schema/func_yaml-schema.json ## Ensures schema file exists.

schema/func_yaml-schema.json: pkg/functions/function.go pkg/functions/function_*.go generate/zz_filesystem_generated.go
	# regenerating schema
	@go run schema/generator/main.go

# remove schema if modified
# when combined with the target 'templates/certs/ca-certificates.crt',
# will rebuild schema if the schema file has been modified.
.PHONY: schema-clean
schema-clean:
	-@FILE="schema/func_yaml-schema.json"; [ -f $$FILE ] && git diff --name-only $$FILE | xargs rm -f

# Checks if the generated function schema file has been modified.
# In CI this is combined with a `make clean` to check that the schema
# embedded fileystem is up-to-date.
.PHONY: schema-modified-check
schema-modified-check: schema
	@if [[ -n $$(git diff -- schema/func_yaml-schema.json) ]]; \
		then echo "\nFunction schema (schema/func_yaml-schema.json) is obsolete, please run 'make schema' and commit the result.\n" >&2; \
	exit 1; fi

.PHONY: certs
certs: certs-clean templates/certs/ca-certificates.crt ## Ensures root certs file exists.

templates/certs/ca-certificates.crt:
	# Updating root certificates
	curl --output templates/certs/ca-certificates.crt https://curl.se/ca/cacert.pem

# remove certs if modified
# when combined with the target 'templates/certs/ca-certificates.crt',
# will rebuild ca-certifictes.crt both if the file is missing or when there
# are local uncommitted modifications.
.PHONY: certs-clean
certs-clean:
	-FILE="templates/certs/ca-certificates.crt"; [ -f $$FILE ] && git diff --name-only $$FILE | xargs rm -f

# Checks that the certificates have no modification.
# In CI this is combined with a `make clean` to check that certificates
# are up-to-date.
.PHONY: certs-modified-check
certs-modified-check: certs
	@if [[ -n $$(git diff -- templates/certs/ca-certificates.crt) ]]; \
		then echo "\nFunction root certs (templates/certs/ca-certificates.crt) are obsolete, please run 'make certs' and commit the result.\n" >&2; \
	exit 1; fi

update-runtimes:  ## Update Function runtime versions (Currently host-built Go functions only)
	cd templates/go/scaffolding/instanced-http && go get -u github.com/lkingland/func-runtime-go/http
	cd templates/go/scaffolding/static-http && go get -u github.com/lkingland/func-runtime-go/http
	cd templates/go/scaffolding/instanced-cloudevents && go get -u github.com/lkingland/func-runtime-go/cloudevents
	cd templates/go/scaffolding/static-cloudevents && go get -u github.com/lkingland/func-runtime-go/cloudevents


