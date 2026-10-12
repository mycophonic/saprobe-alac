# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

# brew's own vendored RuboCop and audit rules: no Ruby toolchain, and plain
# RuboCop would not know the formula cops. brew is not on the hermetic PATH on
# purpose — it is the subject under test, not a substitute for a pinned tool;
# BREW_BIN is captured from the ambient PATH in main.just.

default: style audit

# Homebrew's RuboCop profile over every formula and cask in the repository;
# passes vacuously when there are none. The mutating twin is
# `just do fix homebrew style`.
style: (_banner "lint homebrew" "style")
    #!/usr/bin/env bash
    set -euo pipefail
    {{ _skip_unless_macos }}
    # Same enumeration as `lint just`, modern tap layout ONLY (Formula/ with
    # shards, Casks/): a formula in a legacy location is invisible here.
    # Discovery runs before the brew lookup, so a repo without formulas passes
    # brew-less. The list goes through a file (see lint.just's `just` recipe).
    list=$(mktemp "${TMPDIR:-/tmp}/lint-homebrew.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'Formula/*.rb' 'Formula/**/*.rb' 'Casks/*.rb' 'Casks/**/*.rb' > "$list"
    files=()
    while IFS= read -r -d '' f; do
        # Tracked-but-deleted files are skipped (see lint.just's `just` recipe).
        [ -f "$f" ] || continue
        files+=("$f")
    done < "$list"
    if [ "${#files[@]}" -eq 0 ]; then
        echo "no formulas or casks (Formula/, Casks/) — nothing to lint."
        exit 0
    fi
    {{ _require_brew }}
    # shellcheck disable=SC2154 # BREW_BIN is exported by the canonical .justfile (main.just).
    "$BREW_BIN" style "${files[@]}"

# brew addresses formulas by TAP NAME, never by path: the project declares its
# tap (export LINT_HOMEBREW_TAP := 'user/name') and the working tree is
# symlinked in as that tap for the run, so the audit judges this tree, not a
# clone. Extra flags (CI wants --online) via LINT_HOMEBREW_AUDIT_FLAGS.
audit: (_banner "lint homebrew" "audit")
    #!/usr/bin/env bash
    set -euo pipefail
    {{ _skip_unless_macos }}
    # Same vacuous pass as `style`, through a file. Removed as soon as read:
    # the tap cleanup below takes over the EXIT trap.
    list=$(mktemp "${TMPDIR:-/tmp}/lint-homebrew.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'Formula/*.rb' 'Formula/**/*.rb' 'Casks/*.rb' 'Casks/**/*.rb' > "$list"
    present=""
    while IFS= read -r -d '' f; do
        [ -f "$f" ] && present=1 && break
    done < "$list"
    rm -f "$list"
    if [ -z "$present" ]; then
        echo "no formulas or casks (Formula/, Casks/) — nothing to audit."
        exit 0
    fi
    {{ _require_brew }}
    tap="${LINT_HOMEBREW_TAP:-}"
    case "$tap" in
        */*) ;;
        *)
            echo "LINT_HOMEBREW_TAP is not set (or not 'user/name') — brew audit addresses formulas" >&2
            echo "by tap name, not by path. Export it in the root .justfile, e.g.:" >&2
            echo "  export LINT_HOMEBREW_TAP := 'farcloser/tap'" >&2
            exit 1
            ;;
    esac
    # shellcheck disable=SC2154 # BREW_BIN is exported by the canonical .justfile (main.just).
    link="$("$BREW_BIN" --repository)/Library/Taps/${tap%%/*}/homebrew-${tap##*/}"
    created=""
    if [ -e "$link" ] || [ -L "$link" ]; then
        # Already tapped: proceed only when it is exactly this checkout —
        # auditing a stale clone of the same tap would judge the wrong tree.
        if [ "$(readlink "$link" 2>/dev/null || true)" != "$PWD" ]; then
            echo "tap $tap is already installed at" >&2
            echo "  $link" >&2
            echo "and is not this checkout — untap it (brew untap $tap) or audit there." >&2
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
        fi
    }
    trap cleanup EXIT
    # shellcheck disable=SC2086 # deliberate word-split of the flags knob.
    "$BREW_BIN" audit --strict --tap "$tap" ${LINT_HOMEBREW_AUDIT_FLAGS:-}

# macOS only; elsewhere a LOUD no-op so aggregates keep the module on every
# leg. Injected as a just variable: module recipes cannot share shell
# functions. Twin in fix-homebrew.just.
[private]
_skip_unless_macos := '''
    if [ "$(uname -s)" != 'Darwin' ]; then
        echo "homebrew is macOS-only — nothing to do on this platform (skipped, not failed)."
        exit 0
    fi
'''

# Refuses only when main.just's ambient capture came up empty.
[private]
_require_brew := '''
    if [ -z "${BREW_BIN:-}" ]; then
        echo "brew was not found on your PATH when just started." >&2
        echo "brew is a machine-layer tool — install it outside the repo (or export" >&2
        echo "BREW_BIN=/path/to/brew), then re-run." >&2
        exit 1
    fi
'''
