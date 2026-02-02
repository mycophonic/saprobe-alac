MAKEFILE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
# When included from a project Makefile, MAKEFILE_DIR points to hack/.
# We need the project root instead.
PROJECT_DIR := $(patsubst %/,%,$(dir $(MAKEFILE_DIR)))

VERSION ?= $(shell git -C $(PROJECT_DIR) describe --match 'v[0-9]*' --dirty='.m' --always --tags 2>/dev/null \
	|| echo "no_git_information")
VERSION_TRIMMED := $(VERSION:v%=%)
COMMIT ?= $(shell git -C $(PROJECT_DIR) rev-parse HEAD 2>/dev/null || echo "no_git_information")$(shell \
	if ! git -C $(PROJECT_DIR) diff-index --quiet HEAD 2>/dev/null; then echo .m; fi)
LINT_COMMIT_RANGE ?= main..HEAD
DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
UNAME_S := $(shell uname -s 2>/dev/null || echo Unknown)

ifdef VERBOSE
	VERBOSE_FLAG := -v
	VERBOSE_FLAG_LONG := --verbose
endif

ifndef NO_COLOR
    NC := \033[0m
    GREEN := \033[1;32m
    ORANGE := \033[1;33m
    BLUE := \033[1;34m
    RED := \033[1;31m
endif

# Configurable defaults (override in project Makefile before include)
ICON ?= "🧿"
ORG ?= github.com/mycophonic
ALLOWED_LICENSES ?= Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT
LICENSE_IGNORES ?=
COVER_MIN ?= 0

# Auto-detect Go module presence.
ifneq ($(wildcard $(PROJECT_DIR)/go.mod),)
  HAS_GO := true
else
  HAS_GO := false
endif

# Helpers
recursive_wildcard=$(wildcard $1$2) $(foreach e,$(wildcard $1*),$(call recursive_wildcard,$e/,$2))

define title
	@printf "$(GREEN)____________________________________________________________________________________________________\n" 1>&2
	@printf "$(GREEN)%*s\n" $$(( ( $(shell echo "$(ICON)$(1) $(ICON)" | wc -c ) + 100 ) / 2 )) "$(ICON)$(1) $(ICON)" 1>&2
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(ORANGE)" 1>&2
endef

define footer
	@printf "$(GREEN)> %s: done!\n" "$(1)" 1>&2
	@printf "$(GREEN)____________________________________________________________________________________________________\n$(NC)" 1>&2
endef

help:
	$(call title, $@)
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	$(call footer, $@)

# Tasks
ifeq ($(HAS_GO),true)
lint: lint-go lint-vuln lint-commits lint-mod lint-licenses-all lint-headers lint-yaml lint-shell lint-go-all
else
lint: lint-commits lint-headers lint-yaml lint-shell
endif

ifeq ($(HAS_GO),true)
fix: fix-go-all fix-mod fix-headers ## Automatically fix some issues
else
fix: fix-headers ## Automatically fix some issues
endif

ifeq ($(HAS_GO),true)
all: clean build lint test ## Clean, build, lint, and test everything
else
all: clean lint ## Clean and lint everything
endif

ifeq ($(HAS_GO),true)
test: unit ## Run all tests
unit: test-unit test-unit-race test-unit-bench test-unit-cover ## Run unit tests
else
test: ## Run all tests
	@echo "No Go code detected, skipping tests"
unit: ## Run unit tests
	@echo "No Go code detected, skipping tests"
endif

##########################
# Linting tasks
##########################
lint-go:
	$(call title, $@ $(GOOS))
	@cd $(PROJECT_DIR) \
		&& golangci-lint run $(VERBOSE_FLAG_LONG) ./...
	$(call footer, $@)

lint-vuln:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& govulncheck ./...
	$(call footer, $@)

ifeq ($(CGO_ENABLED),1)
lint-go-all: lint-go
else
lint-go-all:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& GOOS=darwin $(MAKE) lint-go \
		&& GOOS=linux $(MAKE) lint-go \
		&& GOOS=freebsd $(MAKE) lint-go \
		&& GOOS=windows $(MAKE) lint-go
	$(call footer, $@)
endif

lint-yaml:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& yamllint .
	$(call footer, $@)

lint-shell:
	$(call title, $@)
	@files=$$(find $(PROJECT_DIR) -name '*.sh' ! -path '*/tmp/*' ! -path '*/_*' 2>/dev/null); \
        if [ -n "$$files" ]; then shellcheck -a -x $$files; else echo "No shell scripts found, skipping shellcheck"; fi
	$(call footer, $@)

# See https://github.com/andyfeller/gh-ssh-allowed-signers for automation to retrieve contributors keys
lint-commits:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& { git config --unset-all gpg.ssh.allowedSignersFile hack/allowed_signers || true; } \
		&& git config --add gpg.ssh.allowedSignersFile hack/allowed_signers \
		&& git-validation $(VERBOSE_FLAG) -run DCO,short-subject,dangling-whitespace -range "$(LINT_COMMIT_RANGE)"
	$(call footer, $@)

lint-headers:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& ltag -t "./hack/headers" --excludes "bin" --check -v
	$(call footer, $@)

lint-mod:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& go mod tidy --diff
	$(call footer, $@)

# FIXME: go-licenses cannot find LICENSE from root of repo when submodule is imported:
# https://github.com/google/go-licenses/issues/186
# This is impacting gotest.tools
lint-licenses:
	$(call title, $@: $(GOOS))
	@cd $(PROJECT_DIR) \
		&& go-licenses check --include_tests --allowed_licenses=$(ALLOWED_LICENSES) \
		  $(LICENSE_IGNORES) \
		  ./...
	$(call footer, $@)

lint-licenses-all:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& GOOS=darwin $(MAKE) lint-licenses \
		&& GOOS=linux $(MAKE) lint-licenses \
		&& GOOS=freebsd $(MAKE) lint-licenses \
		&& GOOS=windows $(MAKE) lint-licenses
	$(call footer, $@)

##########################
# Automated fixing tasks
##########################
fix-go:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& golangci-lint run --fix
	$(call footer, $@)

fix-go-all:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& GOOS=darwin $(MAKE) fix-go \
		&& GOOS=linux $(MAKE) fix-go \
		&& GOOS=freebsd $(MAKE) fix-go \
		&& GOOS=windows $(MAKE) fix-go
	$(call footer, $@)

fix-mod:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& go mod tidy
	$(call footer, $@)

fix-headers:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& ltag -t "./hack/headers" --excludes "bin"
	$(call footer, $@)

up:
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& go get -u ./...
	$(call footer, $@)

##########################
# Developer environment setup
##########################
# init-dev-system installs platform-specific system dependencies.
# Requires: Homebrew (macOS), apt (Linux), or Chocolatey (Windows).
ifeq ($(OS),Windows_NT)
init-dev-system:
	$(call title, $@)
	@echo "Installing system dependencies (Windows/Chocolatey)..."
	@choco install golang make shellcheck -y
	@pip install yamllint
	$(call footer, $@)
else ifeq ($(UNAME_S),Darwin)
init-dev-system:
	$(call title, $@)
	@echo "Installing system dependencies (macOS/Homebrew)..."
	@brew install golang make yamllint shellcheck
	$(call footer, $@)
else ifeq ($(UNAME_S),Linux)
init-dev-system:
	$(call title, $@)
	@echo "Installing system dependencies (Linux/apt)..."
	@sudo apt-get update -qq
	@sudo apt-get install -qq --no-install-recommends golang make yamllint shellcheck
	$(call footer, $@)
else
init-dev-system:
	$(call title, $@)
	@echo "Unsupported platform: $(UNAME_S)"
	@echo "Please install manually: golang, make, yamllint, shellcheck"
	@exit 1
endif

init-dev: init-dev-system install-dev-tools ## Set up complete development environment

##########################
# Development tools installation
##########################
# Dev tool installs must clear project CGO flags — these tools are unrelated
# third-party binaries and must not inherit hardening flags like -fPIE that
# conflict with Go's default non-PIE link mode for `go install`.
install-dev-gotestsum: export CGO_CFLAGS :=
install-dev-gotestsum: export CGO_CXXFLAGS :=
install-dev-gotestsum: export CGO_CPPFLAGS :=
install-dev-gotestsum: export CGO_LDFLAGS :=
install-dev-gotestsum:
	# gotestsum: 1.13.0 (2025-10-21)
	$(call title, $@)
	@cd $(PROJECT_DIR) \
		&& go install gotest.tools/gotestsum@c4a0df2e75a225d979a444342dd3db752b53619f
	$(call footer, $@)

install-dev-tools: export CGO_CFLAGS :=
install-dev-tools: export CGO_CXXFLAGS :=
install-dev-tools: export CGO_CPPFLAGS :=
install-dev-tools: export CGO_LDFLAGS :=
install-dev-tools: install-dev-gotestsum
	$(call title, $@)
	# 2026-01-31
	# - golangci: v2.8.0
	# - govulncheck: v1.1.4
	# - git-validation: main
	# - ltag: main
	# - go-licenses: v2.0.1
	@cd $(PROJECT_DIR) \
		&& go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@e2e40021c9007020676c93680a36e3ab06c6cd33 \
		&& go install golang.org/x/vuln/cmd/govulncheck@d1f380186385b4f64e00313f31743df8e4b89a77 \
		&& go install github.com/vbatts/git-validation@a8d455533459b620fa656bad095b943e70cede9b \
		&& go install github.com/containerd/ltag@66e6a514664ee2d11a470735519fa22b1a9eaabd \
		&& go install github.com/google/go-licenses/v2@3e084b0caf710f7bfead967567539214f598c0a2
	@echo "Remember to add \$$HOME/go/bin to your path"
	$(call footer, $@)

test-unit:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) -count 1 $(PROJECT_DIR)/...
	$(call footer, $@)

test-unit-bench:
	$(call title, $@)
	@go test $(VERBOSE_FLAG) -count 1 $(PROJECT_DIR)/... -bench=.
	$(call footer, $@)

# Force external linking for race tests. The race detector injects runtime/cgo
# into every package, including those without "import C". CGO_CFLAGS hardening
# flags (-fstack-protector-strong, -fPIE) cause GCC to emit object code
# referencing libc symbols (stderr, etc.). For packages without direct CGO, Go
# uses its internal linker which cannot resolve libc symbols, failing on Linux
# with: runtime/cgo(.text): relocation target stderr not defined
# Forcing -linkmode=external makes GCC perform the final link, resolving libc.
#
# See:
#   https://github.com/golang/go/issues/52690
#   https://github.com/golang/go/issues/54313
#   https://github.com/golang/go/issues/58619
test-unit-race:
	$(call title, $@)
	@CGO_ENABLED=1 go test $(VERBOSE_FLAG) -ldflags="-linkmode=external" $(PROJECT_DIR)/... -race
	$(call footer, $@)

PROF_DIR := $(PROJECT_DIR)/bin/profiles
PROF_DOCS_DIR := $(PROJECT_DIR)/docs/profiles

test-unit-profile: ## Run tests with CPU and memory profiling
	$(call title, $@)
	@mkdir -p $(PROF_DIR) $(PROF_DOCS_DIR)
	@for pkg in $$(go list $(PROJECT_DIR)/...); do \
		name=$$(echo "$$pkg" | sed "s|.*/||"); \
		echo "Profiling $$pkg..."; \
		go test -count 1 $(VERBOSE_FLAG) -o "$(PROF_DIR)/$${name}.test" "$$pkg" \
			-cpuprofile "$(PROF_DIR)/$${name}_cpu.prof" \
			-memprofile "$(PROF_DIR)/$${name}_mem.prof" || true; \
		if [ -s "$(PROF_DIR)/$${name}_cpu.prof" ]; then \
			echo "  CPU profile (top 20):"; \
			go tool pprof -top -nodecount=20 "$(PROF_DIR)/$${name}_cpu.prof" 2>/dev/null || true; \
			go tool pprof -png -nodecount=20 "$(PROF_DIR)/$${name}_cpu.prof" \
				> "$(PROF_DOCS_DIR)/$${name}_cpu.png" 2>/dev/null \
				&& echo "  -> $(PROF_DOCS_DIR)/$${name}_cpu.png" \
				|| echo "  (skipped PNG: graphviz not installed)"; \
		fi; \
		if [ -s "$(PROF_DIR)/$${name}_mem.prof" ]; then \
			echo "  Memory profile — alloc_space (top 20):"; \
			go tool pprof -top -nodecount=20 -alloc_space "$(PROF_DIR)/$${name}_mem.prof" 2>/dev/null || true; \
			go tool pprof -png -nodecount=20 -alloc_space "$(PROF_DIR)/$${name}_mem.prof" \
				> "$(PROF_DOCS_DIR)/$${name}_alloc.png" 2>/dev/null \
				&& echo "  -> $(PROF_DOCS_DIR)/$${name}_alloc.png" \
				|| echo "  (skipped PNG: graphviz not installed)"; \
		fi; \
	done
	@echo "Profiles written to $(PROF_DIR)/"
	@echo "Diagrams written to $(PROF_DOCS_DIR)/"
	@echo "Analyze interactively: go tool pprof <profile>"
	$(call footer, $@)

COVER_DIR := $(PROJECT_DIR)/bin/coverage

test-unit-cover: ## Run tests with coverage reporting
	$(call title, $@)
	@mkdir -p $(COVER_DIR)
	@go test $(VERBOSE_FLAG) -count 1 -coverprofile="$(COVER_DIR)/coverage.out" $(PROJECT_DIR)/...
	@go tool cover -func="$(COVER_DIR)/coverage.out"
	@go tool cover -html="$(COVER_DIR)/coverage.out" -o "$(COVER_DIR)/coverage.html" \
		&& echo "HTML report: $(COVER_DIR)/coverage.html"
	@if [ "$(COVER_MIN)" -gt 0 ] 2>/dev/null; then \
		total=$$(go tool cover -func="$(COVER_DIR)/coverage.out" | grep ^total | awk '{print $$3}' | tr -d '%'); \
		if [ "$$(echo "$$total < $(COVER_MIN)" | bc)" -eq 1 ]; then \
			echo "Coverage $${total}% below minimum $(COVER_MIN)%"; exit 1; \
		fi; \
		echo "Coverage $${total}% meets minimum $(COVER_MIN)%"; \
	fi
	$(call footer, $@)

.PHONY: \
	all \
	lint \
	fix \
	test \
	up \
	unit \
	init-dev init-dev-system \
	install-dev-tools install-dev-gotestsum \
	lint-commits lint-go lint-go-all lint-headers lint-licenses lint-licenses-all lint-mod lint-shell lint-vuln lint-yaml \
	fix-go fix-go-all fix-headers fix-mod \
	test-unit test-unit-race test-unit-bench test-unit-cover test-unit-profile \
	build build-debug build-static install verify clean

# Default target
.DEFAULT_GOAL := help

##########################
# Build infrastructure
##########################

# Auto-detect binaries from cmd/ subdirectories.
BINARIES ?= $(notdir $(wildcard $(PROJECT_DIR)/cmd/*))
BINARY_DIR := $(PROJECT_DIR)/bin

# Go build configuration (override before include as needed)
CGO_ENABLED ?= 0
export CGO_ENABLED

GOFLAGS ?= -tags=netgo,osusergo
export GOFLAGS

GOCMD := go
GOINSTALL := $(GOCMD) install

# Go compiler flags
# -N disables optimizations, -l disables inlining
GCFLAGS_DEBUG := all=-N -l

##########################
# C/C++ hardening (CGO)
##########################
# These are always defined but only take effect when CGO_ENABLED=1.
# Projects needing CGO get hardened builds for free.
#
# References:
#   https://gcc.gnu.org/onlinedocs/gcc/Warning-Options.html
#   https://gcc.gnu.org/onlinedocs/gcc/Optimize-Options.html
#   https://gcc.gnu.org/onlinedocs/gcc/Debugging-Options.html
#   https://gcc.gnu.org/onlinedocs/gcc/Preprocessor-Options.html
#   https://gcc.gnu.org/onlinedocs/gcc/Instrumentation-Options.html
#   https://developers.redhat.com/blog/2018/03/21/compiler-and-linker-flags-gcc
#   https://www.gnu.org/software/libc/manual/html_node/Source-Fortification.html
#   https://news.ycombinator.com/item?id=18874113
#   https://github.com/golang/go/issues/26849

# Windows detection: the OS environment variable is set to "Windows_NT" on all
# modern Windows versions (cmd, PowerShell, Git Bash, MSYS2). Unlike uname,
# this works regardless of which shell Make uses to execute $(shell ...).
# On Windows, force MinGW Makefiles for CMake so that static libraries are
# ABI-compatible with Go's CGO (which uses MinGW GCC, not MSVC).
ifeq ($(OS),Windows_NT)
    CMAKE_GENERATOR := -G "MinGW Makefiles"
else
    CMAKE_GENERATOR :=
endif

# Warning flags
C_WARNING_OPTIONS := -Wall -Werror=format-security

# Optimization
C_OPTIMIZATION_RELEASE := -O2
C_OPTIMIZATION_DEBUG := -O0

# Debug info
C_DEBUGGING_OPTIONS := -grecord-gcc-switches -g

# Security hardening
C_SECURITY_OPTIONS := -fstack-protector-strong -fPIE -D_FORTIFY_SOURCE=2
## Control flow integrity is amd64 only: -mcet -fcf-protection

# C linker flags (passed to ld via CGO_LDFLAGS)
C_LDFLAGS :=

# Linux-specific hardening
ifeq ($(UNAME_S),Linux)
    C_SECURITY_OPTIONS += -fstack-clash-protection
    C_LDFLAGS += -Wl,-z,defs -Wl,-z,relro -Wl,-z,now -Wl,-z,noexecstack
endif

# Composed C flags: release
# -pipe uses pipes instead of temp files for a small speed-up
C_CFLAGS_RELEASE := $(C_WARNING_OPTIONS) $(C_OPTIMIZATION_RELEASE) $(C_SECURITY_OPTIONS) -pipe
C_CPPFLAGS_RELEASE := -D_FORTIFY_SOURCE=2
C_CXXFLAGS_RELEASE := $(C_CFLAGS_RELEASE)

# Composed C flags: debug
C_CFLAGS_DEBUG := $(C_WARNING_OPTIONS) $(C_OPTIMIZATION_DEBUG) $(C_DEBUGGING_OPTIONS) -D_GLIBCXX_ASSERTIONS -pipe
C_CPPFLAGS_DEBUG := -D_GLIBCXX_ASSERTIONS
C_CXXFLAGS_DEBUG := $(C_CFLAGS_DEBUG)

# Export CGO flags for release builds by default.
# Debug builds override these per-target.
export CGO_CFLAGS := $(C_CFLAGS_RELEASE)
export CGO_CPPFLAGS := $(C_CPPFLAGS_RELEASE)
export CGO_CXXFLAGS := $(C_CXXFLAGS_RELEASE)
export CGO_LDFLAGS := $(C_LDFLAGS)

##########################
# Go linker flags
##########################
# -s strips symbol table, -w strips DWARF

LDFLAGS_VERSION = -X $(ORG)/$(NAME)/version.version=$(VERSION) \
    -X $(ORG)/$(NAME)/version.commit=$(COMMIT) \
    -X $(ORG)/$(NAME)/version.date=$(DATE)

# CGO builds need -linkmode=external and -extldflags for PIE.
# Pure Go builds use simpler flags.
# Linker optimization note: CGO_LDFLAGS=-fuse-ld=lld
ifeq ($(CGO_ENABLED),1)
    LDFLAGS_RELEASE = -linkmode=external -s -w $(LDFLAGS_VERSION) -extldflags='-pie'
    LDFLAGS_DEBUG = -linkmode=external $(LDFLAGS_VERSION) -extldflags='-pie'
    LDFLAGS_STATIC = -linkmode=external -s -w $(LDFLAGS_VERSION) -extldflags='-static'
    LDFLAGS_STATIC_DEBUG = -linkmode=external $(LDFLAGS_VERSION) -extldflags='-static'
else
    LDFLAGS_RELEASE = -s -w $(LDFLAGS_VERSION)
    LDFLAGS_DEBUG = $(LDFLAGS_VERSION)
endif

##########################
# Binary build rules
##########################

# Generate per-binary build targets via define/eval.
# Each binary gets: build-<name>, build-debug-<name>, and (if CGO) build-static-<name>.
define make-binary-rules
.PHONY: build-$(1) build-debug-$(1)

build-$(1):
	@echo "Building $(1)..."
	@mkdir -p $(BINARY_DIR)
	$(GOCMD) build -trimpath -buildmode=pie \
		-ldflags '$(LDFLAGS_RELEASE) -X $(ORG)/$(NAME)/version.name=$(1)' \
		-o $(BINARY_DIR)/$(1) ./cmd/$(1)
	@echo "Binary built: $(BINARY_DIR)/$(1)"

build-debug-$(1): export CGO_CFLAGS := $(C_CFLAGS_DEBUG)
build-debug-$(1): export CGO_CPPFLAGS := $(C_CPPFLAGS_DEBUG)
build-debug-$(1): export CGO_CXXFLAGS := $(C_CXXFLAGS_DEBUG)
build-debug-$(1):
	@echo "Building $(1) (debug)..."
	@mkdir -p $(BINARY_DIR)
	$(GOCMD) build -buildmode=pie -gcflags='$(GCFLAGS_DEBUG)' \
		-ldflags '$(LDFLAGS_DEBUG) -X $(ORG)/$(NAME)/version.name=$(1)' \
		-o $(BINARY_DIR)/$(1)-debug ./cmd/$(1)
	@echo "Binary built: $(BINARY_DIR)/$(1)-debug"

ifeq ($(CGO_ENABLED),1)
.PHONY: build-static-$(1)

build-static-$(1):
	@echo "Building $(1) (static)..."
	@mkdir -p $(BINARY_DIR)
	$(GOCMD) build -trimpath \
		-ldflags '$(LDFLAGS_STATIC) -X $(ORG)/$(NAME)/version.name=$(1)' \
		-o $(BINARY_DIR)/$(1)-static ./cmd/$(1)
	@echo "Binary built: $(BINARY_DIR)/$(1)-static"

.PHONY: build-static-debug-$(1)

build-static-debug-$(1): export CGO_CFLAGS := $(C_CFLAGS_DEBUG)
build-static-debug-$(1): export CGO_CPPFLAGS := $(C_CPPFLAGS_DEBUG)
build-static-debug-$(1): export CGO_CXXFLAGS := $(C_CXXFLAGS_DEBUG)
build-static-debug-$(1):
	@echo "Building $(1) (static, debug)..."
	@mkdir -p $(BINARY_DIR)
	$(GOCMD) build -gcflags='$(GCFLAGS_DEBUG)' \
		-ldflags '$(LDFLAGS_STATIC_DEBUG) -X $(ORG)/$(NAME)/version.name=$(1)' \
		-o $(BINARY_DIR)/$(1)-static-debug ./cmd/$(1)
	@echo "Binary built: $(BINARY_DIR)/$(1)-static-debug"
endif
endef

$(foreach bin,$(BINARIES),$(eval $(call make-binary-rules,$(bin))))

build: $(addprefix build-,$(BINARIES)) ## Build all binaries (PIE, release)

build-debug: $(addprefix build-debug-,$(BINARIES)) ## Build all binaries (PIE, debug)

build-static: $(addprefix build-static-,$(BINARIES)) ## Build static binaries (CGO only, Linux)

build-static-debug: $(addprefix build-static-debug-,$(BINARIES)) ## Build static debug binaries (CGO only, Linux)

install: ## Install all binaries to GOPATH/bin
ifneq ($(BINARIES),)
	@$(foreach bin,$(BINARIES),echo "Installing $(bin)..." && $(GOINSTALL) ./cmd/$(bin) && echo "Installed to $$(go env GOPATH)/bin/$(bin)" &&) true
endif

verify: ## Verify all binaries run (--version)
ifneq ($(BINARIES),)
	@$(foreach bin,$(BINARIES),echo "Verifying $(bin)..." && $(BINARY_DIR)/$(bin) --version &&) true
endif

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@echo "Clean complete"
