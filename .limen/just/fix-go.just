# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: code mod

# Only the linter fixes that cannot change what the program does: blank lines
# (wsl_v5), a comment's final period (godot), struct-tag alignment and order
# (tagalign, which tag lookup ignores), and the //nolint directives themselves
# (nolintlint, judging only the linters this pass runs). Every other fixer is
# a pattern rewrite whose author does not promise to preserve meaning, and
# several did not: intrange shortened a `<=` loop by one iteration, gocritic's
# preferFilepathJoin dropped a separator a path needed, usestdlibvars spelled
# labels as stdlib constants. Their findings stay findings, fixed by a person.
# golangci-lint has no per-linter fix switch, so the pass enables only these.
go_fixers := "wsl_v5,godot,tagalign,nolintlint"

# The admitted linter fixes, once per supported platform (see _per-platform in
# lib.just; a fixer only sees a platform's files on that platform's leg), then
# the formatters. The formatters run LAST and to CONVERGENCE: `golangci-lint
# fmt` applies every formatter in one pass and the set is not confluent (one
# formatter's output can be another's work), so a single pass can leave a tree
# `fmt --diff` still rejects — `just fix` green, `just lint` red. Bounded,
# because two formatters undoing each other never settle: loop for the benign
# case, fail loudly for the other. Then the tree is proven to still compile,
# tests included, with `go vet`, the type-check that covers _test.go.
code: (_banner "fix go" "go") (_go-tool "golangci-lint" "github.com/golangci/golangci-lint/v2/cmd/golangci-lint") _golangci-config (_per-platform "build/tools/golangci-lint run --fix --enable-only " + go_fixers + " -c build/golangci.yml")
    #!/usr/bin/env bash
    set -euo pipefail
    max=5
    pass=0
    # Check first: an already-formatted tree costs one --diff and no rewrite.
    while ! build/tools/golangci-lint fmt --diff -c build/golangci.yml >/dev/null 2>&1; do
        pass=$((pass + 1))
        if [ "${pass}" -gt "${max}" ]; then
            echo "formatters did not converge after ${max} passes — they are fighting each other." >&2
            echo "The residual diff below reverses on the next pass; fix the formatter config." >&2
            build/tools/golangci-lint fmt --diff -c build/golangci.yml >&2 || true
            exit 1
        fi
        build/tools/golangci-lint fmt -c build/golangci.yml
    done
    if ! go vet ./...; then
        echo "the fixers left a tree that does not compile (or vet): repair what they broke above, then run again." >&2
        exit 1
    fi

# Tidy go.mod/go.sum, for the module, for tools/ (the analyzers' module) and
# for each isolated tool module (tools/<name>/go.mod, see _go-tool).
mod: (_banner "fix go" "mod")
    #!/usr/bin/env bash
    set -euo pipefail
    go mod tidy
    go -C tools mod tidy
    for m in tools/*/go.mod; do
        [ -f "$m" ] && go -C "${m%/go.mod}" mod tidy
    done

# Update every dependency to its latest version, then tidy.
up: (_banner "fix go" "up") && mod
    go get -u ./...
