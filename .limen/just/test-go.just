# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: unit

# `go test` runs through gotestsum (same exit semantics; GOTESTSUM_FORMAT picks
# the output). -count=1 defeats the result cache. TEST_GO_TIMEOUT overrides the
# timeout from the root .justfile.
#
# A PASSING test's output is hidden twice over (go test drops it without -v;
# gotestsum's default format hides it), so debug prints look like they never
# ran. To see them: GOTESTSUM_FORMAT=standard-verbose just do test go
#
# CGO_ENABLED resolves through lib.just's go_cgo, so a cgo project tests the
# artifact it ships; `race` forces 1 (the detector injects runtime/cgo).

# Unit tests.
unit: (_banner "test go" "unit")
    CGO_ENABLED="{{ go_cgo }}" \
        gotestsum -- -count=1 -timeout "${TEST_GO_TIMEOUT:-10m}" ./...

# Unit tests under the race detector. External linkmode: the detector injects
# runtime/cgo, and hardened CGO_CFLAGS make those objects reference libc
# symbols the internal linker cannot resolve on Linux (golang/go#52690,
# #54313, #58619). macOS ld's "malformed LC_DYSYMTAB" warnings on race builds
# are cosmetic (golang/go#61229).
#
# The detector is a prebuilt runtime Go vendors per platform, absent on
# windows/arm64 among others. Ask the toolchain rather than keep a platform
# list: a probe build of `unsafe` answers in milliseconds. Only the exact
# "-race is not supported" refusal skips — loudly, and passes, since the other
# legs run the detector natively; any other probe failure is real.
race: (_banner "test go" "race")
    #!/usr/bin/env bash
    set -euo pipefail
    export CGO_ENABLED=1
    if ! probe="$(go build -race -o /dev/null unsafe 2>&1)"; then
        if printf '%s\n' "$probe" | grep -q -- '-race is not supported'; then
            echo "race: SKIPPED — no race detector for $(go env GOOS)/$(go env GOARCH); ${probe}" >&2
            exit 0
        fi
        printf '%s\n' "$probe" >&2
        exit 1
    fi
    gotestsum -- -count=1 -timeout "${TEST_GO_TIMEOUT:-10m}" -ldflags=-linkmode=external -race ./...

# Every Fuzz* target for a short budget (TEST_GO_FUZZ_TIME, default 10s).
# Driven by `go test -fuzz`, one target per invocation — only that build
# compiles in the coverage counters. Targets come from -list, not grep, so
# build tags stay Go's call.
#
# The verdict is the crasher, not the exit code: a real finding writes under
# testdata/fuzz/<Target>/ (commit it); the coordinator can also exit 1 with
# "context deadline exceeded" as fuzztime expires mid-iteration, so exit 1
# without a new file is retried once. No Fuzz* targets, or no go.mod at all:
# says so and passes.
fuzz: (_banner "test go" "fuzz")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f go.mod ]; then
        echo "no go.mod — not a Go module, nothing to fuzz (skipped, not failed)."
        exit 0
    fi
    export CGO_ENABLED="{{ go_cgo }}"
    fuzztime="${TEST_GO_FUZZ_TIME:-10s}"
    # Crasher count for a target: the directory does not exist until the first
    # finding, and a missing directory is zero, not an error (pipefail).
    crashers() {
        if [ -d "$1" ]; then find "$1" -type f | wc -l; else echo 0; fi
    }
    found=0
    fail=0
    for pkg in $(go list ./...); do
        targets=$(go test -count=1 -run '^$' -list '^Fuzz' "$pkg" | grep '^Fuzz' || true)
        [ -n "$targets" ] || continue
        dir=$(go list -f '{{{{.Dir}}' "$pkg")
        for target in $targets; do
            found=1
            echo "fuzzing $pkg $target for $fuzztime"
            before=$(crashers "$dir/testdata/fuzz/$target")
            ok=0
            for attempt in 1 2; do
                if go test -count=1 -timeout "${TEST_GO_TIMEOUT:-10m}" \
                    -run "^${target}\$" -fuzz "^${target}\$" -fuzztime "$fuzztime" "$pkg"; then
                    ok=1
                    break
                fi
                after=$(crashers "$dir/testdata/fuzz/$target")
                if [ "$after" -gt "$before" ]; then
                    echo "$target: new crasher written under $dir/testdata/fuzz/$target — commit it as a regression test" >&2
                    break
                fi
                echo "$target: exit without a crasher (attempt $attempt) — coordinator shutdown hiccup, retrying" >&2
            done
            if [ "$ok" -eq 1 ]; then
                echo "PASS: $target"
            else
                fail=1
            fi
        done
    done
    if [ "$found" -eq 0 ]; then
        echo "no Fuzz* targets in this module — nothing to fuzz"
    fi
    exit "$fail"

# Coverage: per-function summary, an HTML report under build/coverage/, and an
# optional minimum gate — export TEST_GO_COVER_MIN := '80' (integer percent)
# from the root .justfile to enforce a floor; unset or 0 reports without gating.
cover: (_banner "test go" "cover")
    #!/usr/bin/env bash
    set -euo pipefail
    export CGO_ENABLED="{{ go_cgo }}"
    dir=build/coverage
    mkdir -p "$dir"
    gotestsum -- -count=1 -timeout "${TEST_GO_TIMEOUT:-10m}" -coverprofile="$dir/coverage.out" ./...
    go tool cover -func="$dir/coverage.out"
    go tool cover -html="$dir/coverage.out" -o "$dir/coverage.html"
    echo "HTML report: $dir/coverage.html"
    min="${TEST_GO_COVER_MIN:-0}"
    # Fail CLOSED on a malformed threshold. `[ "$min" -gt 0 ]` errors on a
    # non-integer (a float like 80.0, a typo like 8O), and swallowing that error
    # would silently skip the whole gate — a mistyped minimum must not disable
    # enforcement. Reject anything that is not a whole number, loudly.
    case "$min" in
        ''|*[!0-9]*)
            echo "TEST_GO_COVER_MIN must be a whole-number percentage (got '$min')." >&2
            exit 2
            ;;
    esac
    if [ "$min" -gt 0 ]; then
        total=$(go tool cover -func="$dir/coverage.out" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
        if [ "${total%%.*}" -lt "$min" ]; then
            echo "coverage ${total}% is below the ${min}% minimum" >&2
            exit 1
        fi
        echo "coverage ${total}% meets the ${min}% minimum"
    fi
