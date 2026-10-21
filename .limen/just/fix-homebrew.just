# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

# The mutating twin of `just do lint homebrew style`: the same brew
# (BREW_STYLE_BIN, defaulting to BREW_BIN) and the same tap link, so what fix
# repairs is what lint reported.

default: style

# Repair what `just do lint homebrew style` reports; passes vacuously when the
# repository carries no formulas or casks.
style: (_banner "fix homebrew" "style")
    #!/usr/bin/env bash
    set -euo pipefail
    {{ _skip_unless_macos }}
    # Same discovery as `lint homebrew style`, through a file (see lint.just's
    # `just` recipe), removed as soon as read: the tap cleanup takes over the
    # EXIT trap.
    list=$(mktemp "${TMPDIR:-/tmp}/fix-homebrew.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'Formula/*.rb' 'Formula/**/*.rb' 'Casks/*.rb' 'Casks/**/*.rb' > "$list"
    files=()
    while IFS= read -r -d '' f; do
        # Tracked-but-deleted files are skipped (see lint.just's `just` recipe).
        [ -f "$f" ] || continue
        files+=("$f")
    done < "$list"
    rm -f "$list"
    if [ "${#files[@]}" -eq 0 ]; then
        echo "no formulas or casks (Formula/, Casks/) — nothing to fix."
        exit 0
    fi
    # shellcheck disable=SC2154 # BREW_BIN is exported by the canonical .justfile (main.just).
    brew="${BREW_STYLE_BIN:-${BREW_BIN:-}}"
    {{ _require_brew }}
    {{ _tap_link }}
    # Each file through the tap link, never the tap's name: see
    # lint-homebrew.just's `style`.
    paths=()
    for f in "${files[@]}"; do
        # shellcheck disable=SC2154 # $link is assigned by the _tap_link snippet above, which the lint lane's dump does not expand.
        paths+=("$link/$f")
    done
    "$brew" style --fix "${paths[@]}"

# Homebrew exists on macOS only: everywhere else this recipe degrades to a
# LOUD no-op — the twin of the variable in lint-homebrew.just; a change here
# must land there too.
[private]
_skip_unless_macos := '''
    if [ "$(uname -s)" != 'Darwin' ]; then
        echo "homebrew is macOS-only — nothing to do on this platform (skipped, not failed)."
        exit 0
    fi
'''

# Refuses when the brew the recipe chose ($brew) is empty — the twin of the
# variable in lint-homebrew.just; a change here must land there too.
[private]
_require_brew := '''
    if [ -z "${brew:-}" ]; then
        echo "brew was not found on your PATH when just started." >&2
        echo "brew is a machine-layer tool — install it outside the repo (or export" >&2
        echo "BREW_BIN=/path/to/brew), then re-run." >&2
        exit 1
    fi
'''

# The working tree as the tap under the chosen brew, sets $tap — the twin of
# the variable in lint-homebrew.just, which says why; a change here must land
# there too.
[private]
_tap_link := '''
    tap="${LINT_HOMEBREW_TAP:-}"
    case "$tap" in
        */*) ;;
        *)
            echo "LINT_HOMEBREW_TAP is not set (or not 'user/name') — brew addresses a tap's formulas" >&2
            echo "by tap name, not by path. Export it in the root .justfile, e.g.:" >&2
            echo "  export LINT_HOMEBREW_TAP := 'farcloser/tap'" >&2
            exit 1
            ;;
    esac
    link="$("$brew" --repository)/Library/Taps/${tap%%/*}/homebrew-${tap##*/}"
    created=""
    if [ -e "$link" ] || [ -L "$link" ]; then
        if [ "$(readlink "$link" 2>/dev/null || true)" != "$PWD" ]; then
            echo "tap $tap is already installed at" >&2
            echo "  $link" >&2
            echo "and is not this checkout — untap it (brew untap $tap) or run there." >&2
            exit 1
        fi
    else
        mkdir -p "$(dirname "$link")"
        ln -s "$PWD" "$link"
        created="$link"
    fi
    cleanup() {
        if [ -n "$created" ]; then
            rm -f "$created"
            # The user directory this run may have made, when nothing else is in it.
            rmdir "$(dirname "$created")" 2>/dev/null || true
        fi
    }
    trap cleanup EXIT
'''
