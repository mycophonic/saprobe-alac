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
    # github.com answers a page fetched from an Actions runner with 429 past
    # lychee's retries, a commit page first. lychee's token never reaches a
    # page: after a page fails it is spent on one API call, GET
    # /repos/<owner>/<repo>, which rescues a bare repository URL, and any URL
    # under a private repository the token reads; under a public repository
    # a path past the repository gets the page's own failure
    # (lychee-lib/src/checker/website.rs, check_github). So lychee gets the
    # token for what it does rescue, and a commit page is excluded from lychee
    # (.limen/lychee.toml) and asked of the API, authenticated, through gh:
    # GET /repos/<owner>/<repo>/commits/<sha> says whether the commit exists.
    # The token is GH_TOKEN in CI (limen-security.yaml), the laptop's own gh
    # login otherwise; it goes to api.github.com and nowhere else, and
    # reaches lychee through its environment (GITHUB_TOKEN), prefixed on each
    # of its invocations and never in an argv.
    token=${GH_TOKEN:-$(gh auth token 2>/dev/null || true)}
    commit_pages='https://github\.com/[^/]+/[^/]+/commit/'
    # The same walk as the check (both configs, their exclude_path), no
    # network: --include wins over the configs' exclusion of the commit pages,
    # so they are listed; grep keeps them alone, whatever else --dump prints.
    commits=$(GITHUB_TOKEN="$token" lychee --no-progress --dump "${args[@]}" --include "$commit_pages" . | { grep -E "^${commit_pages}" || true; } | sort -u)
    checked=0
    missing=0
    if [ -n "$commits" ]; then
        if [ -z "$token" ]; then
            echo "github.com commit links are checked through the API, which needs a token: GH_TOKEN, or gh auth login." >&2
            exit 1
        fi
        while IFS= read -r url; do
            ref=${url#https://github.com/}
            ref=${ref%%[#?]*}
            owner=${ref%%/*}
            repo=${ref#*/}
            repo=${repo%%/*}
            sha=${ref##*/commit/}
            sha=${sha%/}
            sha=${sha%.patch}
            sha=${sha%.diff}
            checked=$((checked + 1))
            # stderr is the verdict (gh: Not Found (HTTP 404)); stdout, the
            # commit, is noise. A verdict is 404 (no such repository) or 422
            # (no such commit); anything else (a 5xx, a cut connection) is
            # asked again twice, as the second pass below does for a page.
            found=0
            for attempt in 1 2 3; do
                if reason=$(gh api "repos/${owner}/${repo}/commits/${sha}" 2>&1 >/dev/null); then
                    found=1
                    break
                fi
                case "$reason" in *"HTTP 404"* | *"HTTP 422"*) break ;; esac
                [ "$attempt" = 3 ] || sleep 5
            done
            if [ "$found" = 0 ]; then
                # grep || true: head closes the pipe once it has its line, and
                # under pipefail a grep killed by that would abort the recipe
                # without its finding.
                where=$({ grep -rnF --exclude-dir=.git -- "$url" . || true; } | head -1 | cut -d: -f1,2)
                echo "[commit] ${url} (${where#./}) | ${reason}"
                missing=$((missing + 1))
            fi
        done <<< "$commits"
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
    GITHUB_TOKEN="$token" lychee --no-progress "${args[@]}" --format json --output "$report" . || status=$?
    if [ "$status" = 0 ]; then
        jq -r '"\(.total) links: \(.successful) ok, \(.excludes) excluded, \(.redirects) redirected"' "$report"
        echo "${checked} github.com commit link(s) asked of the API: $((checked - missing)) ok"
        [ "$missing" = 0 ] || exit 2
        exit 0
    fi
    # 2 is "links failed"; anything else (a config or input error) is final.
    [ "$status" = 2 ] || exit "$status"
    jq -r '(.error_map, .timeout_map) | to_entries[] | .key as $file | .value[]
        | "[\(.status.code // "timeout")] \(.url) (\($file):\(.span.line // 0)) | \(.status.text)"' "$report"
    servers=$(jq '[.error_map[][] | select(.status.code != null and .status.code >= 500 and .status.code < 600)] | length' "$report")
    failures=$(jq '[.error_map[][], .timeout_map[][]] | length' "$report")
    if [ "$servers" = 0 ] || [ "$servers" != "$failures" ] || [ "$missing" != 0 ]; then
        exit 2
    fi
    # The links go back in as a document: a URL given as an input is a page to
    # read links out of, not a link to check.
    jq -r '[.error_map[][] | .url] | unique[] | "<\(.)>"' "$report" > "$scratch/recheck.md"
    echo "only server errors, which lychee does not retry: re-checking $(wc -l < "$scratch/recheck.md" | tr -d ' ') link(s) once, in 60s" >&2
    sleep 60
    GITHUB_TOKEN="$token" lychee --no-progress "${args[@]}" "$scratch/recheck.md"

# Every Go binary the pins name carries its own copy of the standard library, built
# by its upstream with whatever Go they used that day: a fixed net/http or
# crypto/tls reaches us only when THEY rebuild. This scans each such binary
# against the Go vulnerability database (fetched once, see _vulndb in
# lib.just), by the symbols it links, so a known standard-library or module
# vulnerability in a pinned tool is seen the day the advisory lands, whoever
# built it. The binaries the pinned toolchain builds here (.limen/tools/go.mod) are
# the go vuln lane's; this one is for what we did not build. Enumerated from
# the store's links: `hermetic which` answers only for the names this
# repository's pins provide, and `go version -m` tells a Go executable from the rest.
#
# Named, not yet in the default set: its first run found every pinned Go
# binary but the toolchain's own, golangci-lint and limen carrying known
# vulnerabilities, and a red no repository can clear is noise. It joins the
# default set once the pinned tools are built here with the pinned toolchain.
binaries: (_banner "security" "binaries") (_go-tool "govulncheck" "golang.org/x/vuln/cmd/govulncheck") _vulndb
    #!/usr/bin/env bash
    set -euo pipefail
    db=$(cat "$LIMEN_BUILD/vulndb.url")
    bin="$(hermetic root-dir)/bin"
    # A newline-delimited list, not an associative array: macOS ships bash 3.
    seen=$'\n'
    fail=0
    for proxy in "$bin"/*; do
        name=${proxy##*/}
        name=${name%.exe}
        path=$(hermetic which "$name" 2>/dev/null) || continue
        [ -f "$path" ] || continue
        case "$seen" in *$'\n'"$path"$'\n'*) continue;; esac
        seen="$seen$path"$'\n'
        go version -m "$path" >/dev/null 2>&1 || continue
        echo "→ $name"
        # A darwin universal binary (goreleaser ships one) is beyond govulncheck's
        # reader; the other platforms' legs scan that tool's thin binaries, so
        # the darwin leg says so and moves on rather than failing on a format.
        if ! out=$("$LIMEN_BUILD/tools/govulncheck" -db "$db" -mode binary "$path" 2>&1); then
            case "$out" in
                *"unrecognized binary format"*) echo "  skipped: universal binary, scanned on the other platforms";;
                *) printf '%s\n' "$out"; fail=1;;
            esac
        fi
    done
    exit "$fail"
