# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: limen just aqua yaml shell dockerfile commits

# Go linters live in their own submodule: `just do lint go` runs them all,
# `just do lint go <recipe>` (e.g. `just do lint go mod`) runs one.
mod go 'lint-go.just'
mod rust 'lint-rust.just'

# Explicit, never in the default set: brew is macOS-only and machine-layer
# (lint-homebrew.just).
mod homebrew 'lint-homebrew.just'

# First in the default set: the other linters trust the canonical files this
# one verifies. LIMEN_BIN is for the limen repository itself (`go run
# ./cmd/limen`, so the tree is judged by its own enforcer).
limen: (_banner "lint" "limen")
    ${LIMEN_BIN:-limen} check .

# Named, never in the default set: needs the network and an authed gh, and
# judges live GitHub state, not the tree.
github *args: (_banner "lint" "github")
    ${LIMEN_BIN:-limen} github check {{ args }}

just: (_banner "lint" "just")
    #!/usr/bin/env bash
    set -euo pipefail
    # --cached --others --exclude-standard: tracked and untracked-but-not-ignored
    # files. The index half also lists files deleted but unstaged, hence the
    # existence check. Literal names need their '**/' twins: git pathspec
    # literals do not match across directories.
    # The list goes through a file, never a process substitution: on the
    # windows-11-arm runners, bash runs emulated, and a `< <(...)` whose child
    # runs a program intermittently crashes it mid-recipe — exit 4 or 127, no
    # output. A pipeline has not been seen to. Same in every shared recipe
    # (design/WINDOWS-ARM-EXIT-4.md).
    list=$(mktemp "${TMPDIR:-/tmp}/lint-just.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'justfile' '**/justfile' 'Justfile' '**/Justfile' '.justfile' '**/.justfile' '*.just' > "$list"
    while IFS= read -r -d '' f; do
        [ -f "$f" ] || continue
        just --fmt --check --justfile "$f"
    done < "$list"

# godolint — hadolint as pure Go (https://github.com/forkcloser/godolint) —
# over every Dockerfile in the repository; passes vacuously when there are
# none, so it can sit in the default set for every repo. Rule exceptions
# belong next to the instruction they excuse, as inline
# `# hadolint ignore=DLxxxx` pragmas — not in the recipe.
dockerfile: (_banner "lint" "dockerfile") (_go-tool "godolint" "github.com/forkcloser/godolint/cmd/godolint")
    #!/usr/bin/env bash
    set -euo pipefail
    # Same enumeration as `lint just` (through a file, see there), over both
    # Dockerfile and Containerfile spellings. '*.Dockerfile' needs no '**/'
    # twin: a wildcard already matches across directories.
    list=$(mktemp "${TMPDIR:-/tmp}/lint-dockerfile.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'Dockerfile' '**/Dockerfile' 'Dockerfile.*' '**/Dockerfile.*' '*.Dockerfile' \
        'Containerfile' '**/Containerfile' 'Containerfile.*' '**/Containerfile.*' '*.Containerfile' > "$list"
    files=()
    while IFS= read -r -d '' f; do
        # Tracked-but-deleted files are skipped (see `lint just`).
        [ -f "$f" ] || continue
        files+=("$f")
    done < "$list"
    # godolint runs shellcheck on RUN lines when it finds one on PATH — the
    # pinned one, here.
    [ ${#files[@]} -eq 0 ] || "$LIMEN_BUILD/tools/godolint" "${files[@]}"

yaml: (_banner "lint" "yaml") (_go-tool "yamlfmt" "github.com/forkcloser/yamlfmt/cmd/yamlfmt")
    "$LIMEN_BUILD/tools/yamlfmt" -conf .limen/.yamlfmt -lint

shell: (_banner "lint" "shell")
    #!/usr/bin/env bash
    set -euo pipefail
    # A file is a shell script if it declares itself one, by either signal:
    #   - a .sh/.bash extension → a missing shebang is then a lint error (SC2148), or
    #   - a shebang on line 1 for a dialect shellcheck lints (sh/bash/dash/ksh —
    #     not zsh, which shellcheck cannot check) → covers extension-less scripts.
    # Filtering on the shebang alone would silently skip an *.sh that forgot its shebang.
    # No mapfile (bash 4+): build the arrays with read loops, portable to bash 3.2.
    # Process count is the cost that matters: under git-bash a process is tens
    # of milliseconds, and a head and a grep per file made this recipe six
    # times slower on windows than on linux. The loop below is builtins only,
    # and one awk per hundred candidates (windows caps a command line at 32K)
    # reads their first lines, in the C locale: the candidates include binaries,
    # and macOS awk in a UTF-8 locale dies on their first bytes ("towc:
    # multibyte conversion failure"). No \b in awk regexes — the word boundary
    # is spelled out. Every list goes through a file in the scratch directory
    # (see `lint just`), under a dot-name no recipe body below can take.
    # mktemp gets an explicit template: macOS ignores $TMPDIR.
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/lint-shell.XXXXXX")
    trap 'rm -rf "$tmpdir"' EXIT
    git ls-files -z --cached --others --exclude-standard > "$tmpdir/.files"
    files=()
    candidates=()
    while IFS= read -r -d '' f; do
        # Tracked-but-deleted files are skipped (see `lint just`).
        [ -f "$f" ] || continue
        case "$f" in
            *.sh|*.bash) files+=("$f"); continue ;;
        esac
        candidates+=("$f")
    done < "$tmpdir/.files"
    for ((i = 0; i < ${#candidates[@]}; i += 100)); do
        LC_ALL=C awk '
            FNR == 1 {
                # %c with 0, not "\0" in the format: a NUL inside an awk
                # string ends it, and the names would run together unterminated.
                if ($0 ~ /^#!.*[^[:alnum:]_](bash|dash|ksh|sh)([^[:alnum:]_]|$)/) printf "%s%c", FILENAME, 0
                nextfile
            }' "${candidates[@]:i:100}"
    done > "$tmpdir/.shebang"
    while IFS= read -r -d '' f; do files+=("$f"); done < "$tmpdir/.shebang"
    [ ${#files[@]} -eq 0 ] || shellcheck --rcfile .limen/.shellcheckrc "${files[@]}"
    # Shebang recipes inside justfiles are shell scripts too: extract each body
    # from the JSON dump and shellcheck it. Interpolations (just syntax, not
    # shell) are masked as INTERP. One dump of the root justfile covers imports
    # and every nested `mod`.
    recipes=()
    dump=$(just --dump --dump-format json)
    # jq keeps the shebang recipes and emits each as a record — name, body,
    # then a \x1e separator, a byte no source carries — and ONE awk writes the
    # bodies out, rather than a base64 and a head per recipe. The separator
    # is built with implode, never spelled as a byte or an escape in this text: the recipe
    # is itself in the dump, and a JSON escape in its text decodes to the
    # byte and splits its own record. The tr is load-bearing: native windows
    # jq writes CRLF. Every record after the first starts with the newline jq
    # prints after the separator. ':' is illegal in windows filenames; the
    # name is only a label, so module separators are flattened. The directory
    # reaches awk through ENVIRON, never -v: -v processes escapes, and a
    # windows temp path's backslashes are eaten (\l becomes l).
    # Each body ends with one line naming the root's exports as assigned
    # (`: "${NAME:=}"`): just puts them in every recipe's environment, but a
    # body read alone references LIMEN_BUILD unassigned (SC2154 under
    # check-unassigned-uppercase). Appended, so the reported lines stay the
    # recipe's own, and behind an SC2317 directive, since after a body's
    # last `exit` the line is unreachable.
    printf '%s' "$dump" | jq -r '
            ((.assignments // {}) | to_entries | map(select(.value.export) | "\"${" + .key + ":=}\"")
                | if length == 0 then [] else ["# shellcheck disable=SC2317", ": " + join(" ")] end) as $exports
            | def recipes_of(prefix):
                ((.recipes // {}) | to_entries[]
                    | (.value.body
                       | map(map(if type == "string" then . else "INTERP" end) | join(""))) as $lines
                    | select(($lines[0] // "") | test("^#!.*\\b(bash|dash|ksh|sh)\\b"))
                    | prefix + .key, ($lines + $exports | join("\n")), ([30] | implode)),
                ((.modules // {}) | to_entries[]
                    | .key as $mod | .value | recipes_of(prefix + $mod + "::"));
            recipes_of("")' | tr -d '\r' | LINT_SHELL_DIR="$tmpdir" awk '
            BEGIN { RS = "\036" }
            {
                sub(/^\n/, "")
                n = index($0, "\n")
                if (n == 0) next
                name = substr($0, 1, n - 1)
                gsub(/:/, "_", name)
                out = ENVIRON["LINT_SHELL_DIR"] "/" name
                printf "%s", substr($0, n + 1) > out
                close(out)
                print name
            }' > "$tmpdir/.recipes"
    while IFS= read -r name; do recipes+=("$tmpdir/$name"); done < "$tmpdir/.recipes"
    # SC1010 is excluded for recipe bodies only: `just do <task>` is our shared
    # namespace, and shellcheck misreads the argument "do" as the shell keyword.
    # The constant-word checks (SC2050/SC2078/SC2157/SC2194) are excluded for
    # recipe bodies only: a masked INTERP is constant to shellcheck but dynamic
    # in reality, so those hits can only be false positives of the masking.
    [ ${#recipes[@]} -eq 0 ] || shellcheck --rcfile .limen/.shellcheckrc --exclude=SC1010,SC2050,SC2078,SC2157,SC2194 "${recipes[@]}"

# Validate commit hygiene — DCO sign-off, subject length, dangling whitespace —
# over a commit range (default: everything ahead of the upstream default branch;
# pass one explicitly otherwise, e.g. `just do lint commits v1.0.0..HEAD`).
commits range="": (_banner "lint" "commits") (_go-tool "git-validation" "github.com/forkcloser/git-validation/cmd/git-validation")
    #!/usr/bin/env bash
    set -euo pipefail
    range="{{ range }}"
    if [ -z "$range" ] && [ -n "${GITHUB_BASE_REF:-}" ]; then
        # GitHub Actions pull request: validate against the PR's actual base
        # branch, whatever it targets. Requires actions/checkout with
        # fetch-depth: 0 — the default shallow checkout has no base refs (and
        # only the synthetic merge commit as "history": vacuously green).
        if git rev-parse --verify -q "origin/$GITHUB_BASE_REF" >/dev/null 2>&1; then
            range="origin/${GITHUB_BASE_REF}..HEAD"
        fi
    fi
    if [ -z "$range" ]; then
        # Default to the commits this branch adds over the upstream default
        # branch. origin/HEAD is only set when `git remote set-head` ran, so
        # fall back through the common default-branch names; with no upstream
        # at all, validate the full history.
        for ref in origin/HEAD origin/main origin/master; do
            if git rev-parse --verify -q "$ref" >/dev/null 2>&1; then
                range="$ref..HEAD"
                break
            fi
        done
    fi
    # git-validation asks git for every commit's signature status, so git
    # verifies each signature: without the signers file and a gpg it prints
    # an error per commit that reads like a failure. GIT_CONFIG_* hands git
    # both without writing .git/config.
    n=0
    if [ -f .lint-signers ]; then
        export GIT_CONFIG_KEY_0=gpg.ssh.allowedSignersFile GIT_CONFIG_VALUE_0=.lint-signers
        n=1
    fi
    if [ -n "${GPG_BIN:-}" ]; then
        export "GIT_CONFIG_KEY_$n=gpg.program" "GIT_CONFIG_VALUE_$n=$GPG_BIN"
        n=$((n + 1))
    fi
    export GIT_CONFIG_COUNT=$n
    if [ -n "$range" ]; then
        "$LIMEN_BUILD/tools/git-validation" --run DCO,short-subject,dangling-whitespace --range "$range"
    else
        "$LIMEN_BUILD/tools/git-validation" --run DCO,short-subject,dangling-whitespace
    fi
    # .lint-signers lists everyone who authors here, email and key, in git's
    # allowed-signers format: every commit in the range must be signed by a
    # key it lists for the committer (%G? is G). Not a security boundary,
    # since a pull request can edit the file: a forcing function, so an author
    # lists themselves. Skipped: merges, which author nothing (GitHub makes
    # the pull request checkout's and the update-branch button's), and a
    # GitHub App's commits (Renovate, the checksum-update App), authored by
    # the bot and committed by GitHub. A human's commit that GitHub made, a
    # web-UI edit, fails. The file is passed per command: nothing is written
    # to .git/config. Without a range (no upstream) there is nothing to check.
    if [ -f .lint-signers ] && [ -n "$range" ]; then
        commits=$(git log --no-merges --format='%H%x09%ae%x09%ce' "$range")
        unsigned=0
        while IFS=$'\t' read -r sha author committer; do
            [ -n "$sha" ] || continue
            case "$author" in
                *'[bot]@users.noreply.github.com') [ "$committer" = noreply@github.com ] && continue ;;
            esac
            status=$(git -c gpg.ssh.allowedSignersFile=.lint-signers log -1 --format=%G? "$sha" 2>/dev/null || true)
            if [ "$status" != G ]; then
                echo "$(git log -1 --format='%h %s' "$sha"): not signed by a key .lint-signers lists for ${committer}" >&2
                unsigned=1
            fi
        done <<< "$commits"
        if [ "$unsigned" -ne 0 ]; then
            echo "sign your commits, and list yourself in .lint-signers: <email> namespaces=\"git\" <public key>" >&2
            exit 1
        fi
    elif [ ! -f .lint-signers ]; then
        echo "warning: no .lint-signers: commit signatures are not checked — list every author, <email> namespaces=\"git\" <public key>" >&2
    fi

# Verify .hermetic/aqua-checksums.json is in sync with .hermetic/hermetic.yaml — the
# working tree's pair, exactly as it stands: git state (staged, committed, neither) is
# irrelevant to coherence. Recipes judge the working tree; git answers
# enumeration and history questions only.
aqua: (_banner "lint" "aqua")
    #!/usr/bin/env bash
    set -euo pipefail
    # update-checksum has no read-only form, so regenerate, compare against a
    # snapshot, and restore (needs network): the lint observes, never mutates —
    # `just do fix aqua` is the mutating twin. mktemp gets an explicit template
    # because macOS mktemp ignores $TMPDIR.
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/lint-aqua.XXXXXX")
    trap 'rm -rf "$tmpdir"' EXIT
    had=""
    if [ -f .hermetic/aqua-checksums.json ]; then
        cp .hermetic/aqua-checksums.json "$tmpdir/snapshot"
        had=1
    fi
    # Restore the working tree from the snapshot on EVERY exit — success, drift,
    # a hermetic failure (set -e), or an interrupt. update-checksum mutates
    # .hermetic/aqua-checksums.json in place, and a lint must never leave that behind; an
    # inline restore only runs on the paths it is written on, so it belongs in the
    # trap. Installed AFTER the snapshot so `had` is known — restoring before that
    # would wrongly rm a file we had not yet copied.
    # Trap-invoked functions aren't seen as invoked, so this is false-flagged as
    # dead code (SC2329) — https://github.com/koalaman/shellcheck/wiki/SC2329.
    # shellcheck disable=SC2329
    restore() {
        if [ -n "$had" ]; then
            cp "$tmpdir/snapshot" .hermetic/aqua-checksums.json
        else
            rm -f .hermetic/aqua-checksums.json
        fi
        rm -rf "$tmpdir"
    }
    trap restore EXIT
    # --log-level warn: the per-package INFO lines are noise when nothing is wrong.
    {{ agent_home }} hermetic --log-level warn -c .hermetic/hermetic.yaml update-checksum --prune
    if [ -n "$had" ] && cmp -s .hermetic/aqua-checksums.json "$tmpdir/snapshot"; then
        exit 0
    fi
    echo ".hermetic/aqua-checksums.json is out of sync with .hermetic/hermetic.yaml — run 'just do fix aqua'." >&2
    exit 1

# The files derived from a pin (.converge, book/tooling.md) are what the
# project's `converge` recipe makes of the tree as it stands. Named, never in
# the default set: the recipe is the project's and may fetch a toolchain, so a
# project that declares derived files names this lane in its own lint.
# Vacuous without a .converge.
converge: (_banner "lint" "converge")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f .converge ]; then
        echo "no .converge — no derived files declared, nothing to check (skipped, not failed)."
        exit 0
    fi
    # mktemp gets an explicit template because macOS mktemp ignores $TMPDIR.
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/lint-converge.XXXXXX")
    trap 'rm -rf "$tmpdir"' EXIT
    # The declaration, held to what the checksum workflow accepts: one tracked
    # regular file under git's own name for it, or, ending in a slash, a
    # directory that holds tracked files, under git's own name too; either
    # outside the directories that are git's, GitHub's or limen's.
    # Whether git tracks exactly this name: literal, so a name is no
    # pattern, and NUL-ended, so a name git would quote is compared as it is.
    tracked_as() {
        printf '%s\0' "$1" > "$tmpdir/want"
        git --literal-pathspecs ls-files -z -- "$1" > "$tmpdir/got" 2> /dev/null || return 1
        cmp -s "$tmpdir/want" "$tmpdir/got"
    }
    : > "$tmpdir/declared"
    : > "$tmpdir/declared-dirs"
    while IFS= read -r line || [ -n "$line" ]; do
        path="${line%$'\r'}"
        case "$path" in
            '' | '#'*) continue ;;
        esac
        case "$path" in
            .git | .git/* | .github | .github/* | .limen | .limen/* | .hermetic | .hermetic/*)
                echo ".converge: $path is not a path a derived file may have." >&2
                exit 1
                ;;
        esac
        case "$path" in
            */)
                # Literal, so a name is no pattern. The first path git lists
                # carries the directory's own spelling or the entry was not
                # it; a path git refuses outright lists nothing.
                git --literal-pathspecs ls-files -z -- "$path" > "$tmpdir/under" 2> /dev/null || : > "$tmpdir/under"
                first=""
                IFS= read -r -d '' first < "$tmpdir/under" || true
                if [ -z "$first" ] || [ "${first#"$path"}" = "$first" ] || [ ! -d "$path" ] || [ -L "${path%/}" ]; then
                    echo ".converge: $path is not a directory of tracked files under that exact name." >&2
                    exit 1
                fi
                printf '%s\n' "$path" >> "$tmpdir/declared-dirs"
                continue
                ;;
        esac
        if ! tracked_as "$path" || [ ! -f "$path" ] || [ -L "$path" ]; then
            echo ".converge: $path is not a tracked regular file under that exact name." >&2
            exit 1
        fi
        printf '%s\n' "$path" >> "$tmpdir/declared"
    done < .converge
    under_declared_dir() {
        while IFS= read -r dir; do
            case "$1" in
                "$dir"?*) return 0 ;;
            esac
        done < "$tmpdir/declared-dirs"
        return 1
    }
    # The regular files git sees under the declared directories, tracked or
    # not, ignored ones aside: what the checksum workflow would carry.
    # NUL-separated, a name being anything but that.
    listed() {
        : > "$1"
        while IFS= read -r dir; do
            git --literal-pathspecs ls-files -z --cached --others --exclude-standard -- "$dir" > "$tmpdir/raw"
            while IFS= read -r -d '' path; do
                if [ -f "$path" ] && [ ! -L "$path" ]; then
                    printf '%s\0' "$path" >> "$1"
                fi
            done < "$tmpdir/raw"
        done < "$tmpdir/declared-dirs"
    }
    # The lint observes, never mutates: the declared files, and every file
    # under a declared directory, are restored as they were found on every
    # exit, a failing recipe's included; what the recipe created there goes.
    mkdir -p "$tmpdir/snapshot"
    while IFS= read -r path; do
        mkdir -p "$tmpdir/snapshot/$(dirname "$path")"
        cp "$path" "$tmpdir/snapshot/$path"
    done < "$tmpdir/declared"
    listed "$tmpdir/dirs-before"
    while IFS= read -r -d '' path; do
        mkdir -p "$tmpdir/snapshot/${path%/*}"
        cp "$path" "$tmpdir/snapshot/$path"
    done < "$tmpdir/dirs-before"
    # No link stands on the way to a path, or in its place: where the recipe
    # left one, at the file or at any directory above it, a removal or a copy
    # would go through it, into whatever it points at. No tracked path runs
    # through a link, so any that is there is the recipe's.
    # shellcheck disable=SC2329 # called from restore
    make_way() {
        local way="" part
        local -a parts
        IFS=/ read -r -a parts <<< "$1"
        for part in "${parts[@]}"; do
            way="${way:+$way/}$part"
            if [ -L "$way" ]; then
                rm -f "$way"
            fi
        done
        rm -rf "$1"
        case "$1" in
            */*) mkdir -p "${1%/*}" ;;
        esac
    }
    # shellcheck disable=SC2329 # trap-invoked
    restore() {
        while IFS= read -r path; do
            make_way "$path"
            cp "$tmpdir/snapshot/$path" "$path"
        done < "$tmpdir/declared"
        listed "$tmpdir/dirs-now"
        while IFS= read -r -d '' path; do
            if [ ! -f "$tmpdir/snapshot/$path" ]; then
                rm -f "$path"
                rmdir -p "${path%/*}" 2> /dev/null || true
            fi
        done < "$tmpdir/dirs-now"
        while IFS= read -r -d '' path; do
            make_way "$path"
            cp "$tmpdir/snapshot/$path" "$path"
        done < "$tmpdir/dirs-before"
        rm -rf "$tmpdir"
    }
    trap restore EXIT
    # NUL-ended, as the checksum workflow reads it: a name git would quote
    # (a space, a non-ASCII letter) is then the name itself.
    git status --porcelain -z --untracked-files=all > "$tmpdir/before"
    # shellcheck disable=SC2329 # called from the loop below
    was_before() {
        while IFS= read -r -d '' seen; do
            if [ "$seen" = "$1" ]; then
                return 0
            fi
        done < "$tmpdir/before"
        return 1
    }
    # By name, as every shared recipe calls it: the pinned just, first on the
    # PATH. Its own path, pasted here unquoted, is backslashes on Windows,
    # which bash strips.
    just converge
    git status --porcelain -z --untracked-files=all > "$tmpdir/after"
    failed=0
    while IFS= read -r path; do
        if ! cmp -s "$path" "$tmpdir/snapshot/$path"; then
            echo "$path is not what 'just converge' makes of this tree — run it and commit the result." >&2
            failed=1
        fi
    done < "$tmpdir/declared"
    # A declared directory is derived whole: a file changed, a file the
    # recipe makes that is not there, a file there the recipe no longer
    # makes.
    listed "$tmpdir/dirs-after"
    while IFS= read -r -d '' path; do
        if [ ! -f "$path" ] || [ -L "$path" ]; then
            echo "$path is no longer made by 'just converge' — run it and commit the result." >&2
            failed=1
        elif ! cmp -s "$path" "$tmpdir/snapshot/$path"; then
            echo "$path is not what 'just converge' makes of this tree — run it and commit the result." >&2
            failed=1
        fi
    done < "$tmpdir/dirs-before"
    while IFS= read -r -d '' path; do
        if [ ! -f "$tmpdir/snapshot/$path" ]; then
            echo "$path is made by 'just converge' and is not in this tree — run it and commit the result." >&2
            failed=1
        fi
    done < "$tmpdir/dirs-after"
    # What the recipe touched beside what is declared: the status lines that
    # were not there before it ran. Under a declared directory anything but
    # a regular file or a deletion is one too: the checksum workflow carries
    # nothing else.
    while IFS= read -r -d '' entry; do
        path="${entry:3}"
        if was_before "$entry" || grep -Fxq -- "$path" "$tmpdir/declared"; then
            continue
        fi
        if under_declared_dir "$path"; then
            if [ -L "$path" ] || { [ -e "$path" ] && [ ! -f "$path" ]; }; then
                echo "'just converge' made $path, which is not a regular file: the checksum workflow carries regular files alone." >&2
                failed=1
            fi
            continue
        fi
        echo "'just converge' changed $path, which .converge does not declare: the checksum workflow commits declared files alone." >&2
        failed=1
    done < "$tmpdir/after"
    exit "$failed"
