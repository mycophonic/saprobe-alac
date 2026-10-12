# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

# The compile-only reports; bench and profile are named, being slow and,
# for profile, writing under build/.
default: bce escape

# Every bounds check the compiler could not eliminate. Informational: never
# fails, native arch only (BCE is arch-specific).
bce: (_banner "perf go" "bce")
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Bounds Check Elimination Report"
    echo "================================"
    echo ""
    # Keep compiler diagnostics only (file.go:line:col:): the build's other
    # chatter — package banners, a CGO link's `ld: warning` — must not count.
    output=$(go build -gcflags='-d=ssa/check_bce/debug=1' ./... 2>&1 | grep -E '\.go:[0-9]+:[0-9]+: ' || true)
    # Generic instantiations compile in the CALLER's package, so the compiler
    # also reports positions inside GOROOT (slices.SortFunc & co) and the module
    # cache (a generic dependency). Real checks in the binary, but not this
    # module's code to change — keep the module's own files only. Positions are
    # relative to the module root, so anything outside it starts with "../" (or
    # is absolute).
    external=$(echo "$output" | grep -cE '^(\.\./|/)' || true)
    output=$(echo "$output" | grep -vE '^(\.\./|/)' || true)
    if [ "$external" != 0 ]; then
        echo "(${external} in generic instantiations from GOROOT/module cache omitted — not this module's code)"
        echo ""
    fi
    if [ -z "$output" ]; then
        echo "No bounds checks detected (BCE fully eliminated)."
    else
        total=$(echo "$output" | wc -l | tr -d ' ')
        echo "Total: $total bounds checks"
        echo ""
        echo "By file:"
        echo "$output" | sed 's/:.*$//' | sort | uniq -c | sort -rn
        echo ""
        echo "Details:"
        echo "$output" | sort
    fi

# Escape-analysis and inlining report — both come from the same compiler pass
# (-gcflags=-m): values the compiler moved to the heap (the prime source of GC
# pressure) and functions it refused to inline, with the reason. Informational
# like bce: never fails, native architecture only.
escape: (_banner "perf go" "escape")
    #!/usr/bin/env bash
    set -euo pipefail
    # Same filters as bce: compiler diagnostics only, then drop positions
    # inside GOROOT / the module cache (generic instantiations compiled into
    # this module's packages).
    output=$(go build -gcflags='-m' ./... 2>&1 | grep -E '\.go:[0-9]+:[0-9]+: ' || true)
    external=$(echo "$output" | grep -cE '^(\.\./|/)' || true)
    output=$(echo "$output" | grep -vE '^(\.\./|/)' || true)
    heap=$(echo "$output" | grep -E 'escapes to heap|moved to heap' || true)
    noinline=$(echo "$output" | grep 'cannot inline' || true)
    echo "Escape Analysis Report"
    echo "======================"
    if [ "$external" != 0 ]; then
        echo "(${external} lines in generic instantiations from GOROOT/module cache omitted — not this module's code)"
        echo ""
    fi
    if [ -z "$heap" ]; then
        echo "No heap escapes."
    else
        echo "Total: $(echo "$heap" | wc -l | tr -d ' ') heap escapes"
        echo ""
        echo "By file:"
        echo "$heap" | sed 's/:.*$//' | sort | uniq -c | sort -rn
        echo ""
        echo "$heap" | sort
    fi
    echo ""
    echo "Inlining Failures"
    echo "================="
    if [ -z "$noinline" ]; then
        echo "None."
    else
        echo "Total: $(echo "$noinline" | wc -l | tr -d ' ') functions the compiler could not inline"
        echo ""
        echo "$noinline" | sort
    fi

# Benchmarks, with allocation stats. -run '^$' deselects unit tests so only
# benchmarks run — `just do test go` already covers the tests themselves.
# CGO_ENABLED resolves through lib.just's go_cgo, so a cgo project measures
# the artifact it ships.
bench: (_banner "perf go" "bench")
    CGO_ENABLED="{{ go_cgo }}" \
        go test -count=1 -timeout "${TEST_GO_TIMEOUT:-10m}" -run '^$' -bench . -benchmem ./...

# CPU and memory profiles per package, pprof top-20 printed, PNG call graphs
# via the pinned `dot` (forkcloser/dot: graphviz as a single Go binary — WASM
# via wazero — tagged, which is what lets it be a tool directive that moves).
# Informational: a failing package is still profiled past. Raw under
# build/profiles/, PNG under build/profiles-docs/.
profile: (_banner "perf go" "profile") (_go-tool "dot" "github.com/forkcloser/dot/cmd/dot")
    #!/usr/bin/env bash
    set -euo pipefail
    export CGO_ENABLED="{{ go_cgo }}"
    prof=build/profiles
    docs=build/profiles-docs
    mkdir -p "$prof" "$docs"
    # Rendered from pprof's -dot text rather than pprof's own -png, which
    # shells out to a dot it expects on PATH. -o with a pre-created file: under
    # a sandbox the WASM runtime can overwrite files but not always create
    # them. Paths stay cwd-relative for the same reason (the runtime preopens
    # only the working directory).
    render() {
        src=$1; png=$2; shift 2
        go tool pprof -dot -nodecount=20 "$@" "$src" > "$png.dot" 2>/dev/null
        : > "$png"
        build/tools/dot -Tpng -o "$png" "$png.dot"
        rm -f "$png.dot"
        echo "  -> $png"
    }
    for pkg in $(go list ./...); do
        # One directory per package keyed by the FULL import path: flattening
        # collides (example/a__b vs example/a/b, '_' being legal in import
        # paths). Paths stay cwd-relative for the WASM dot (see render).
        mkdir -p "$prof/$pkg" "$docs/$pkg"
        echo "Profiling $pkg..."
        go test -count=1 -o "$prof/$pkg/pkg.test" "$pkg" \
            -cpuprofile "$prof/$pkg/cpu.prof" \
            -memprofile "$prof/$pkg/mem.prof" || true
        if [ -s "$prof/$pkg/cpu.prof" ]; then
            echo "  CPU profile (top 20):"
            go tool pprof -top -nodecount=20 "$prof/$pkg/cpu.prof" 2>/dev/null || true
            render "$prof/$pkg/cpu.prof" "$docs/$pkg/cpu.png"
        fi
        if [ -s "$prof/$pkg/mem.prof" ]; then
            echo "  Memory profile — alloc_space (top 20):"
            go tool pprof -top -nodecount=20 -alloc_space "$prof/$pkg/mem.prof" 2>/dev/null || true
            render "$prof/$pkg/mem.prof" "$docs/$pkg/alloc.png" -alloc_space
        fi
    done
    echo "Profiles written to $prof/, diagrams to $docs/"
