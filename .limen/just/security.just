# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

# Security scans are not linters. A linter judges the tree, and the same tree
# gets the same verdict tomorrow; a scan judges the tree against a database
# that moves on its own, so a green turns red with nothing pushed. Hence a
# family of their own, apart from lint, and a CI lane of their own
# (.github/workflows/security.yaml), so a new advisory reddens one check that
# says what it is, and a red here is answered by a bump, not by a code change.

# The Go scans live in their own submodule: `just do security go` runs them
# all, `just do security go <recipe>` (vuln) runs one.
mod go 'security-go.just'

# A submodule's recipe IS addressable as a dependency (go::default); only a
# bare module path (go) is not.
default: links go::default

# Here and not in lint: a link's verdict is the web's, not the tree's, and moves
# without a push, exactly a scan's shape; in lint it reddened every verify leg
# and gate on one host's bad minute, five times over.

# Check documentation links with lychee (respects .gitignore, skips hidden files).
links: (_banner "security" "links")
    #!/usr/bin/env bash
    set -euo pipefail
    # Canonical exclusions (and their rationale) live in .limen/lychee.toml; a
    # repository adds its own in a root .lint-links.toml. Both must be passed
    # explicitly — any --config disables lychee's ./lychee.toml auto-discovery —
    # and lychee merges them, concatenating the exclude lists.
    args=(--config .limen/lychee.toml)
    if [ -f .lint-links.toml ]; then
        args+=(--config .lint-links.toml)
    fi
    # WORKAROUND, lychee through 0.24.2 (lychee-lib/src/retry.rs): a rejected
    # status is judged retryable only when it is 429, so a 5xx is never
    # retried whatever max_retries says, and one GitHub 503 blip fails the
    # lane. Server errors alone get a second pass, re-checked once a minute
    # later; any other failure is a finding at once. Drop this when lychee
    # retries 5xx itself.
    scratch=$(mktemp -d "${TMPDIR:-/tmp}/security-links.XXXXXX")
    trap 'rm -rf "$scratch"' EXIT
    report="$scratch/report.json"
    status=0
    lychee --no-progress "${args[@]}" --format json --output "$report" . || status=$?
    if [ "$status" = 0 ]; then
        jq -r '"\(.total) links: \(.successful) ok, \(.excludes) excluded, \(.redirects) redirected"' "$report"
        exit 0
    fi
    # 2 is "links failed"; anything else (a config or input error) is final.
    [ "$status" = 2 ] || exit "$status"
    jq -r '(.error_map, .timeout_map) | to_entries[] | .key as $file | .value[]
        | "[\(.status.code // "timeout")] \(.url) (\($file):\(.span.line // 0)) | \(.status.text)"' "$report"
    servers=$(jq '[.error_map[][] | select(.status.code != null and .status.code >= 500 and .status.code < 600)] | length' "$report")
    failures=$(jq '[.error_map[][], .timeout_map[][]] | length' "$report")
    if [ "$servers" = 0 ] || [ "$servers" != "$failures" ]; then
        exit 2
    fi
    # The links go back in as a document: a URL given as an input is a page to
    # read links out of, not a link to check.
    jq -r '[.error_map[][] | .url] | unique[] | "<\(.)>"' "$report" > "$scratch/recheck.md"
    echo "only server errors, which lychee does not retry: re-checking $(wc -l < "$scratch/recheck.md" | tr -d ' ') link(s) once, in 60s" >&2
    sleep 60
    lychee --no-progress "${args[@]}" "$scratch/recheck.md"

# Every Go binary aqua pins carries its own copy of the standard library, built
# by its upstream with whatever Go they used that day: a fixed net/http or
# crypto/tls reaches us only when THEY rebuild. This scans each such binary
# against the Go vulnerability database (fetched once, see _vulndb in
# lib.just), by the symbols it links, so a known standard-library or module
# vulnerability in a pinned tool is seen the day the advisory lands, whoever
# built it. The binaries the pinned toolchain builds here (tools/go.mod) are
# the go vuln lane's; this one is for what we did not build. Enumerated from
# aqua's proxies: `aqua which` answers only for the names this repository's
# config provides, and `go version -m` tells a Go executable from the rest.
#
# Named, not yet in the default set: its first run found every pinned Go
# binary but the toolchain's own, golangci-lint and limen carrying known
# vulnerabilities, aqua included, and a red no repository can clear is
# noise. It joins the default set once the pinned tools are built here with
# the pinned toolchain and aqua's own pin is current.
binaries: (_banner "security" "binaries") (_go-tool "govulncheck" "golang.org/x/vuln/cmd/govulncheck") _vulndb
    #!/usr/bin/env bash
    set -euo pipefail
    db=$(cat build/vulndb.url)
    bin="$(aqua root-dir)/bin"
    # A newline-delimited list, not an associative array: macOS ships bash 3.
    seen=$'\n'
    fail=0
    for proxy in "$bin"/*; do
        name=${proxy##*/}
        name=${name%.exe}
        path=$(aqua which "$name" 2>/dev/null) || continue
        [ -f "$path" ] || continue
        case "$seen" in *$'\n'"$path"$'\n'*) continue;; esac
        seen="$seen$path"$'\n'
        go version -m "$path" >/dev/null 2>&1 || continue
        echo "→ $name"
        # A darwin universal binary (goreleaser ships one) is beyond govulncheck's
        # reader; the other platforms' legs scan that tool's thin binaries, so
        # the darwin leg says so and moves on rather than failing on a format.
        if ! out=$(build/tools/govulncheck -db "$db" -mode binary "$path" 2>&1); then
            case "$out" in
                *"unrecognized binary format"*) echo "  skipped: universal binary, scanned on the other platforms";;
                *) printf '%s\n' "$out"; fail=1;;
            esac
        fi
    done
    exit "$fail"
