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
# BREW_BIN is captured from the ambient PATH in main.just. `style` (and its
# fixing twin) and `audit` may each run from another brew than the machine's:
# BREW_STYLE_BIN and BREW_AUDIT_BIN, exported by the project's .justfile,
# default to BREW_BIN. The machine's brew cannot run under a session sandbox
# (it writes under its own prefix and caches) and refuses a tap it already has
# installed, so without them `audit` is CI-only in a tap repository. `test`
# stays on the machine's, which installs for real.

default: style audit

# Homebrew's RuboCop profile over the repository's formulas and casks, each
# file named through the tap link: brew reads a tap's own RuboCop
# configuration (tap_rubocop_style.yml) only for files it can place in one
# tap; the same files by their path in the tree get Homebrew's own
# Library/.rubocop.yml, a different verdict from the one CI and `audit` get.
# Never the tap's name: brew takes a tap directory as a path and globs every
# *.sh beneath it for shellcheck, which it then installs into its own prefix,
# a brew that cannot run under a sandbox and a lint the shell lane already
# does. Passes vacuously when there are none. The mutating twin is
# `just do fix homebrew style`.
style: (_banner "lint homebrew" "style")
    #!/usr/bin/env bash
    set -euo pipefail
    {{ _skip_unless_macos }}
    # Same enumeration as `lint just`, modern tap layout ONLY (Formula/ with
    # shards, Casks/): a formula in a legacy location is invisible here.
    # Discovery runs before the brew lookup, so a repo without formulas passes
    # brew-less. The list goes through a file (see lint.just's `just` recipe),
    # removed as soon as read: the tap cleanup below takes over the EXIT trap.
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
    rm -f "$list"
    if [ "${#files[@]}" -eq 0 ]; then
        echo "no formulas or casks (Formula/, Casks/) — nothing to lint."
        exit 0
    fi
    # shellcheck disable=SC2154 # BREW_BIN is exported by the canonical .justfile (main.just).
    brew="${BREW_STYLE_BIN:-${BREW_BIN:-}}"
    {{ _require_brew }}
    {{ _tap_link }}
    paths=()
    for f in "${files[@]}"; do
        # shellcheck disable=SC2154 # $link is assigned by the _tap_link snippet above, which the lint lane's dump does not expand.
        paths+=("$link/$f")
    done
    "$brew" style "${paths[@]}"

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
    # shellcheck disable=SC2154 # BREW_BIN is exported by the canonical .justfile (main.just).
    brew="${BREW_AUDIT_BIN:-${BREW_BIN:-}}"
    {{ _require_brew }}
    {{ _tap_link }}
    # shellcheck disable=SC2086 # deliberate word-split of the flags knob.
    # shellcheck disable=SC2154 # $tap is assigned by the _tap_link snippet above, which the lint lane's dump does not expand.
    "$brew" audit --strict --tap "$tap" ${LINT_HOMEBREW_AUDIT_FLAGS:-}

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

# Refuses when the brew the recipe chose ($brew) is empty: main.just's ambient
# capture came up empty and the project exported no BREW_STYLE_BIN or
# BREW_AUDIT_BIN. Twin in fix-homebrew.just.
[private]
_require_brew := '''
    if [ -z "${brew:-}" ]; then
        echo "brew was not found on your PATH when just started." >&2
        echo "brew is a machine-layer tool — install it outside the repo (or export" >&2
        echo "BREW_BIN=/path/to/brew), then re-run." >&2
        exit 1
    fi
'''

# The working tree as the tap, under the chosen brew ($brew): brew addresses a
# tap's formulas by tap name and reads the tap's own RuboCop configuration only
# for files it can place in a tap, so `style` and `audit` alike register this
# checkout as LINT_HOMEBREW_TAP for the run, a symlink under the brew's own
# Taps, removed at exit when this run made it. A tap already installed from
# elsewhere is refused: judging a stale clone is the wrong tree. Sets $tap and
# $link. Twin in fix-homebrew.just.
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
