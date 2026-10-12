# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: code vet mod licenses nilaway

# golangci-lint, built here by the pinned toolchain from tools/golangci-lint
# (its own module, so its analyzers' dependencies stay upstream's; see _go-tool
# in lib.just), on the configuration limen-lint-go renders from the baseline
# and the project's .lint-go.yaml (see _golangci-config in lib.just: the file
# lives under build/, so `-c` names it and golangci ignores any root
# .golangci.yml), then once per supported platform (see _per-platform:
# platform-specific files are only analyzed for the GOOS/GOARCH that builds
# them). Built with the toolchain it analyzes, it cannot skew from it — the
# reason the prebuilt binary and its lockstep check are gone.
# The formatter is build-graph-independent and runs once. The last step
# enforces the suppression doctrine (book/per-language.md, "silencing a
# finding"): a finding is silenced by its rule, never by its linter.
code: (_banner "lint go" "go") (_go-tool "golangci-lint" "github.com/golangci/golangci-lint/v2/cmd/golangci-lint") _golangci-config (_per-platform "build/tools/golangci-lint run -c build/golangci.yml") _suppression-check
    build/tools/golangci-lint fmt --diff -c build/golangci.yml

# Polices the shape the book requires (per-language.md, "silencing a
# finding"): nolintlint checks a directive's form, not which linter it names,
# and bag linters (revive, gosec, staticcheck) must be silenced per rule.
# Tracked Go files only.
[private]
_suppression-check:
    #!/usr/bin/env bash
    set -euo pipefail
    fail=0
    # ban <message> <pattern> [<allowed-pattern>]: every tracked Go line matching
    # <pattern> and not <allowed-pattern> is a violation.
    ban() {
        local hits
        hits="$(git grep -nE "$2" -- '*.go' || true)"
        if [ -n "$hits" ] && [ -n "${3:-}" ]; then hits="$(printf '%s\n' "$hits" | grep -vE "$3" || true)"; fi
        if [ -n "$hits" ]; then printf '%s\n' "$hits" >&2; echo "$1" >&2; fail=1; fi
    }
    ban 'blanket revive suppression is banned — use //revive:disable-next-line:<rule> (or a disable/enable block)' \
        'nolint:[a-z, ]*revive'
    ban 'blanket gosec suppression is banned — use gosec'"'"'s own // #nosec G### -- reason (on its own comment line when combined with a //nolint)' \
        'nolint:[a-z, ]*gosec'
    ban '#nosec must name its rule(s): // #nosec G### -- reason' \
        '#nosec' '#nosec( G[0-9]{3})+( --|[[:space:]]*$)'
    ban 'staticcheck suppressions must name the check in the reason: //nolint:staticcheck // SA####: reason' \
        'nolint:[a-z, ]*staticcheck' '(^|[^A-Z])(SA|ST|S|QF)[0-9]{4}([^0-9]|$)'
    ban 'bare //nolint is banned — name the linter (or, for a bag linter, the rule)' \
        '//nolint(:all)?([^a-z:]|$)'
    # A revive directive naming a rule the rendered configuration turns off
    # silences nothing, and nolintlint does not read revive's syntax: this is
    # the only place a stale one is ever reported. The lists come through a
    # capture and a here-string, never a process substitution (see lint.just's
    # `just` recipe), which also stops a failing query from passing silently.
    disabled=$(${LIMEN_LINT_GO_BIN:-limen-lint-go} disabled revive)
    while IFS= read -r rule; do
        [ -n "$rule" ] || continue
        ban "revive's ${rule} is off in the baseline: this directive silences nothing — remove it (or move the finding to the linter that owns the check now)" \
            "//revive:(disable|enable)[a-z-]*:${rule}([^a-z-]|$)"
    done <<< "$disabled"
    # Same for a //nolint naming a linter the configuration turns off:
    # nolintlint judges a directive only for a linter that runs.
    disabled=$(${LIMEN_LINT_GO_BIN:-limen-lint-go} disabled linters)
    while IFS= read -r linter; do
        [ -n "$linter" ] || continue
        ban "${linter} is off in the configuration: naming it in //nolint silences nothing — remove it from the directive" \
            "//nolint:([a-z0-9_]+,)*${linter}([^a-z0-9_]|$)"
    done <<< "$disabled"
    # The baseline exempts _test.go from gosec, and nolintlint never reads
    # gosec's own directive: a #nosec in a test file silences nothing. Tied
    # to that exclusion: drop this if the baseline ever lints tests with gosec.
    hits="$(git grep -nE '#nosec' -- '*_test.go' || true)"
    if [ -n "$hits" ]; then
        printf '%s\n' "$hits" >&2
        echo 'tests are exempt from gosec in the baseline: this #nosec silences nothing — remove it' >&2
        fail=1
    fi
    exit "$fail"

# The two `go vet` analyzers golangci-lint cannot run: its package loader never
# hands analyzers a package's non-Go files, so asmdecl (assembly against the Go
# declarations it implements) and buildtag (constraints in non-Go files) report
# nothing through it whatever the govet configuration says — even
# `enable-all`. Broken amd64 assembly passed `lint go` on the amd64 CI legs
# for exactly that reason. Scoped to those two on purpose: `go vet` reads
# neither the rendered configuration nor //nolint, so any analyzer that overlaps with
# golangci's govet would re-report findings a project deliberately silenced,
# and the same exception would then have to be written twice. Every
# supported platform, like the rest (see _per-platform in lib.just); vet
# type-checks without an assembler or C toolchain, so each leg is a few
# seconds. Note `go test` is not a backstop here: the vet subset it runs
# excludes both analyzers.
vet: (_banner "lint go" "vet") (_per-platform "go vet -asmdecl -buildtag ./...")

# go.mod/go.sum tidiness, for the module and for tools/ (the Go-built tools'
# own module — see _go-tool in lib.just). Fails printing the diff; `just do fix
# go mod` repairs.
mod: (_banner "lint go" "mod")
    #!/usr/bin/env bash
    set -euo pipefail
    go mod tidy -diff
    go -C tools mod tidy -diff
    # The isolated tool modules too (tools/<name>/go.mod, see _go-tool).
    for m in tools/*/go.mod; do
        [ -f "$m" ] && go -C "${m%/go.mod}" mod tidy -diff
    done

# NilAway (go.uber.org/nilaway), Uber's nil-panic analyzer: not a golangci-lint
# linter and never to become one (its false positives are by design), so a
# lane of its own, built from tools/nilaway (a module of its own, pinned by
# pseudo-version) and run once per supported platform like the other
# analyzers (see _per-platform in lib.just). limen-lint-go carries its flags
# from the baseline and the project's .lint-go.yaml: -include-pkgs bounds
# the analysis to the module, the exclude lists are the only suppression it
# has (no per-line directive). Blocking by default; a project carrying a
# backlog sets nilaway.blocking: false in its overlay and the findings print
# without failing the lane.
nilaway: (_banner "lint go" "nilaway") (_go-tool "nilaway" "go.uber.org/nilaway/cmd/nilaway")
    #!/usr/bin/env bash
    set -euo pipefail
    flags=$(${LIMEN_LINT_GO_BIN:-limen-lint-go} flags nilaway)
    mode=$(${LIMEN_LINT_GO_BIN:-limen-lint-go} mode nilaway)
    fail=0
    # The platform loop of _per-platform, inline: a leg's failure must not
    # stop the others, and the verdict depends on the mode.
    if [ "{{ go_cgo }}" = "1" ]; then
        echo "CGO_ENABLED=1: cross-platform runs are impossible (foreign-platform package" >&2
        echo "loading needs that platform's C toolchain) — native platform only." >&2
        # shellcheck disable=SC2086 # the flags are a list by contract
        build/tools/nilaway $flags ./... || fail=1
    else
        platforms=(darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64)
        for platform in "${platforms[@]}"; do
            echo "→ GOOS=${platform%/*} GOARCH=${platform#*/}"
            # shellcheck disable=SC2086 # the flags are a list by contract
            CGO_ENABLED=0 GOOS="${platform%/*}" GOARCH="${platform#*/}" build/tools/nilaway $flags ./... || fail=1
        done
    fi
    if [ "$fail" = 1 ] && [ "$mode" != blocking ]; then
        echo "nilaway: findings above are informational for this project (nilaway.blocking is false in .lint-go.yaml)" >&2
        exit 0
    fi
    exit "$fail"

# Dependency license compliance, once per supported platform (see _per-platform in
# lib.just: each platform pulls its own dependencies). The allowed licenses
# are the baseline's (.limen/lint-go.yaml) and the ignored modules
# (typically the false positives of google/go-licenses#186) the project's
# .lint-go.yaml `licenses` section; limen-lint-go prints the flags for both
# (LIMEN_LINT_GO_BIN: see _golangci-config in lib.just).
#
# GOROOT is passed explicitly because go-licenses recognizes stdlib packages by
# their location under GOROOT, reading its default from its own build. _go-tool
# compiles it with the pinned toolchain, so the two agree by construction; the
# explicit value keeps that true even when a project overrides GOROOT.
#
# The module's own path is ignored by the lane: the lane judges the dependency
# graph, the repository's own LICENSE is the `license` rule's, and go-licenses
# classifies a license by its text, so the Closed-source notice limen writes
# failed every package of such a module.
licenses: (_banner "lint go" "licenses") (_go-tool "go-licenses" "github.com/google/go-licenses/v2") (_per-platform 'GOROOT="$(go env GOROOT)" build/tools/go-licenses check --include_tests --ignore "$(go list -m)" $(${LIMEN_LINT_GO_BIN:-limen-lint-go} flags licenses) ./...')

# Unreachable functions, by whole-program call-graph analysis (deeper than the
# per-package `unused` linter in golangci). -test keeps library-only repos
# analyzable (test binaries serve as roots) and stops dead code from hiding
# behind test-only callers. Informational: native only, not in the default set.
deadcode: (_banner "lint go" "deadcode") (_go-tool "deadcode" "golang.org/x/tools/cmd/deadcode")
    build/tools/deadcode -test ./...
