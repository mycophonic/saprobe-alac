# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe, go_tool_build and agent_home (see lib.just).
import 'lib.just'

# hermetic's GitHub API calls (a package's tag list, its release assets) are
# limited per address when unauthenticated, and one checksum update makes
# one per package: a laptop pinning one tool across nine repositories hit the
# 403 on the second (the download then 404s on an unset version). The laptop's
# own gh login authenticates them when it has one and nothing set the
# variable; setup-hermetic sets it in CI. HERMETIC_GITHUB_TOKEN is the one
# variable hermetic reads a token from. Prefixed on the API-calling commands
# only, never exported to a recipe's other children.
aqua_api := 'HERMETIC_GITHUB_TOKEN="${HERMETIC_GITHUB_TOKEN:-$(gh auth token 2>/dev/null || true)}" hermetic'

# --- Project tooling (see book/tooling.md). Every recipe takes the tool as it
#     is pinned: an aqua slug exactly as in .hermetic/hermetic.yaml (goreleaser/goreleaser),
#     or a Go package path exactly as in .limen/tools/go.mod
#     (golang.org/x/tools/cmd/deadcode). ---
#
# The two are told apart by the first path element: a Go module root is a
# host and always carries a dot; a GitHub owner never can. .hermetic/hermetic.yaml holds no
# go_install package any more (book/tooling.md), so there is no overlap.
#
# The mutating recipes end by really installing the tool, never by linking:
# a FULL `hermetic install` for an aqua package (links verify nothing — lazy pulls
# are a CI economy, wrong at the moment a pin changes; on a warm machine the
# full install is incremental), and a native build into .build/tools/ for a
# Go tool (the same build _go-tool does before a recipe runs it). Either way a
# bad pin fails here, not at first use.
#
# The recipes edit the project's own pins in .hermetic/hermetic.yaml, never the
# canonical set it imports (.limen/hermetic.yaml, limen's): a project pin of a
# canonical tool overrides limen's version, and removing it falls back to it.

# hermetic on the project's own pins alone: a sibling copy of the manifest with
# the import commented out, run from outside the repository — run inside it, a
# command the copy lacks is resolved through the real manifest, and the
# library rewrites .limen/hermetic.yaml (and `generate` skips a canonical tool as
# already declared). HERMETIC_NO_GLOBAL_CONFIG keeps the toolset's own pins
# out as well: the binary derives the global config from its root, so
# unsetting the variable would not. The copy, import restored, then replaces
# the manifest.
project_aqua := '''
    project_aqua() {
        local copy scratch rc=0
        copy=$(mktemp .hermetic/hermetic.yaml.XXXXXX)
        scratch=$(mktemp -d "${TMPDIR:-/tmp}/limen-tools.XXXXXX")
        sed 's|^\([[:space:]]*\)- import: \(\.\./\.limen/hermetic\.yaml\)[[:space:]]*$|\1# limen-import: \2|' .hermetic/hermetic.yaml > "$copy"
        (cd "$scratch" && HERMETIC_NO_GLOBAL_CONFIG=true HERMETIC_GITHUB_TOKEN="${HERMETIC_GITHUB_TOKEN:-$(gh auth token 2>/dev/null || true)}" hermetic -c "$OLDPWD/$copy" "$@") || rc=$?
        rmdir "$scratch"
        if [ "$rc" -eq 0 ]; then
            sed 's|^\([[:space:]]*\)# limen-import: |\1- import: |' "$copy" > "$copy.restored"
            mv "$copy.restored" .hermetic/hermetic.yaml
        fi
        rm -f "$copy" "$copy.restored"
        return "$rc"
    }
'''

# Add a new tool at its latest version or at one given, e.g. `just do tools add junegunn/fzf`.
[positional-arguments]
add pkg version="": (_banner "tools" "add")
    #!/usr/bin/env bash
    set -euo pipefail
    pkg="$1"; version="${2:-}"
    # Regex-escape the name: slugs and package paths carry dots, and an
    # unescaped '.' would match a sibling (`foo/li.r` vs `foo/liar`). ']' and
    # '\' are absent from both and trip BSD sed.
    esc=$(printf '%s' "$pkg" | sed 's/[.^$*+?()[{}|]/\\&/g')
    first=$pkg; first=${first%%/*}
    if [[ $first == *.* ]]; then
        if grep -qE "^(tool[[:space:]]+)?[[:space:]]*${esc}([[:space:]]|$)" .limen/tools/go.mod 2>/dev/null; then
            echo "${pkg} is already a tool directive in .limen/tools/go.mod — use 'just do tools update <command>' or 'just do tools set ${pkg} <version>'." >&2
            exit 1
        fi
        go -C .limen/tools get -tool "${pkg}@${version:-latest}"   # the directive
        go -C .limen/tools mod tidy
        {{ go_tool_build }} "$pkg"           # REAL build: a bad pin fails here, not at first use
        exit 0
    fi
    if grep -qE "^[[:space:]]*-[[:space:]]*name:[[:space:]]*${esc}([[:space:]]|@|$)" .hermetic/hermetic.yaml; then
        echo "${pkg} is already in .hermetic/hermetic.yaml — use 'just do tools update <command>' or 'just do tools set ${pkg} <version>'." >&2
        exit 1
    fi
    {{ project_aqua }}
    entry=$(project_aqua generate "$pkg")   # the entry at the latest version
    # An http package with no version source (a local-registry entry that reads
    # a static host, rustup say) generates with no version at all: the one
    # given goes on, and none given is an error, not a pin with a hole.
    if [ -n "$version" ]; then
        entry=$(printf '%s\n' "$entry" | sed "1s#@[^[:space:]]*##; 1s#\$#@${version}#")
    elif ! printf '%s\n' "$entry" | head -1 | grep -q '@'; then
        echo "${pkg} lists no versions (an http package): give one, 'just do tools add ${pkg} <version>'." >&2
        exit 1
    fi
    # The entry goes right above the import, at its indent: the resolver takes a
    # package's first declaration, so below it the entry would be shadowed.
    tmp=$(mktemp .hermetic/hermetic.yaml.XXXXXX)
    trap 'rm -f "$tmp"' EXIT
    if ! ENTRY=$entry awk '
        /^[[:space:]]*- import: \.\.\/\.limen\/hermetic\.yaml[[:space:]]*$/ && !done {
            indent = $0; sub(/-.*/, "", indent)
            n = split(ENVIRON["ENTRY"], lines, "\n")
            for (i = 1; i <= n; i++) if (lines[i] != "") print indent lines[i]
            done = 1
        }
        { print }
        END { if (!done) exit 1 }
    ' .hermetic/hermetic.yaml > "$tmp"; then
        echo ".hermetic/hermetic.yaml does not import ../.limen/hermetic.yaml — run 'limen fix' first." >&2
        exit 1
    fi
    mv "$tmp" .hermetic/hermetic.yaml
    {{ agent_home }} {{ aqua_api }} -c .hermetic/hermetic.yaml update-checksum --prune      # record its checksum (and drop stale ones)
    {{ agent_home }} hermetic -c .hermetic/hermetic.yaml install                      # REAL install: a bad pin fails here, not at first use

# Set an existing tool to an exact version, e.g. `just do tools set goreleaser/goreleaser v2.0.0`.
[positional-arguments]
set pkg version: (_banner "tools" "set")
    #!/usr/bin/env bash
    set -euo pipefail
    pkg="$1"; version="$2"
    # See `add` for the escaping.
    esc=$(printf '%s' "$pkg" | sed 's/[.^$*+?()[{}|]/\\&/g')
    first=$pkg; first=${first%%/*}
    if [[ $first == *.* ]]; then
        # An isolated module (.limen/tools/<name>/go.mod, see _go-tool in lib.just)
        # holds the directive for the tool that builds as <name>.
        name=${pkg##*/}
        if [[ $name =~ ^v[0-9]+$ ]]; then name=${pkg%/*}; name=${name##*/}; fi
        if grep -qE "^(tool[[:space:]]+)?[[:space:]]*${esc}([[:space:]]|$)" ".limen/tools/${name}/go.mod" 2>/dev/null; then
            go -C ".limen/tools/${name}" get -tool "${pkg}@${version}"   # moves the pin
            go -C ".limen/tools/${name}" mod tidy
            mkdir -p "$LIMEN_BUILD/tools" && GOOS='' GOARCH='' CGO_ENABLED=0 go -C ".limen/tools/${name}" build -o "$LIMEN_BUILD/tools/" "$pkg"
            exit 0
        fi
        if ! grep -qE "^(tool[[:space:]]+)?[[:space:]]*${esc}([[:space:]]|$)" .limen/tools/go.mod 2>/dev/null; then
            echo "${pkg} is not a tool directive in .limen/tools/go.mod — add it with 'just do tools add ${pkg}'." >&2
            exit 1
        fi
        go -C .limen/tools get -tool "${pkg}@${version}"   # moves the pin
        go -C .limen/tools mod tidy
        {{ go_tool_build }} "$pkg"                   # REAL build: a bad pin fails here, not at first use
        exit 0
    fi
    if ! grep -qE "^[[:space:]]*-[[:space:]]*name:[[:space:]]*${esc}@" .hermetic/hermetic.yaml; then
        echo "${pkg} is not pinned in .hermetic/hermetic.yaml — add it with 'just do tools add ${pkg} ${version}'." >&2
        exit 1
    fi
    # Explicit sibling template: macOS mktemp ignores $TMPDIR, and a sibling
    # makes the mv an atomic same-filesystem rename.
    tmp=$(mktemp .hermetic/hermetic.yaml.XXXXXX)
    trap 'rm -f "$tmp"' EXIT
    sed -E "s#(^[[:space:]]*-[[:space:]]*name:[[:space:]]*${esc})@[^[:space:]]*#\1@${version}#" .hermetic/hermetic.yaml > "$tmp"
    mv "$tmp" .hermetic/hermetic.yaml
    {{ agent_home }} {{ aqua_api }} -c .hermetic/hermetic.yaml update-checksum --prune      # new checksum in, replaced version's out
    {{ agent_home }} hermetic -c .hermetic/hermetic.yaml install                      # REAL install: a bad pin fails here, not at first use

# Takes the COMMAND name (`just`, `limen`, `deadcode`), not the package: `aqua
# update` resolves a command to the project's pin, local registry included,
# and a tool directive is found by the command it builds as. With no argument
# every tool moves: each project aqua pin and the registries (`hermetic update`), then
# each directive at its latest — one `go get` for all of them, so the module
# graph is solved once — and every directive is rebuilt. Renovate does this
# one tool at a time; the no-argument form is for moving everything by hand
# and reading one diff. (The blank line below keeps this paragraph out of
# `just --list`; only the line right above the recipe is its doc.)

# Update one tool to its latest version (`just do tools update golangci-lint`), or every tool with no argument.
[positional-arguments]
update command="": (_banner "tools" "update")
    #!/usr/bin/env bash
    set -euo pipefail
    command="${1:-}"
    # Every directive in .limen/tools/go.mod: a `tool` line and a `tool (…)` block are
    # both walked; a `//` comment line inside the block is not a directive.
    # (A read loop, not mapfile: macOS ships bash 3. A capture and a
    # here-string, never a process substitution: see lint.just's `just`.)
    directives=$(awk '
        /^tool[[:blank:]]*\(/ { block = 1; next }
        block && /^\)/ { block = 0; next }
        /^tool[[:blank:]]/ { print $2 }
        block && NF && $1 !~ /^\/\// { print $1 }
    ' .limen/tools/go.mod 2>/dev/null || true)
    pkgs=()
    while IFS= read -r p; do
        [ -n "$p" ] || continue
        pkgs+=("$p")
    done <<< "$directives"
    # A directive builds as its last path element, except that go strips a /vN
    # major suffix (a directive ending in /v2 builds as the element before it —
    # see _go-tool).
    name_of() { local n=${1##*/}; if [[ $n =~ ^v[0-9]+$ ]]; then n=${1%/*}; n=${n##*/}; fi; printf '%s' "$n"; }
    # The isolated modules, .limen/tools/<name>/go.mod (see _go-tool in lib.just):
    # one directive each, moved and rebuilt in their own module.
    isolated_update() {
        local dir=$1 p
        p=$(awk '/^tool[[:blank:]]/ { print $2; exit }' "$dir/go.mod")
        [ -n "$p" ] || return 0
        go -C "$dir" get -tool "${p}@latest"
        go -C "$dir" mod tidy
        mkdir -p "$LIMEN_BUILD/tools" && GOOS='' GOARCH='' CGO_ENABLED=0 go -C "$dir" build -o "$LIMEN_BUILD/tools/" "$p"
    }
    {{ project_aqua }}
    if [ -z "$command" ]; then
        project_aqua update               # every project pin and the registries, to the latest
        {{ agent_home }} {{ aqua_api }} -c .hermetic/hermetic.yaml update-checksum --prune      # new checksums in, replaced versions' out
        {{ agent_home }} hermetic -c .hermetic/hermetic.yaml install                      # REAL install: a bad pin fails here, not at first use
        if [ ${#pkgs[@]} -gt 0 ]; then
            go -C .limen/tools get -tool "${pkgs[@]/%/@latest}"   # one solve for all of them
            go -C .limen/tools mod tidy
            {{ go_tool_build }} "${pkgs[@]}"              # REAL build of every directive
        fi
        for m in .limen/tools/*/go.mod; do
            [ -f "$m" ] && isolated_update "${m%/go.mod}"
        done
        exit 0
    fi
    if [ -f ".limen/tools/${command}/go.mod" ]; then
        isolated_update ".limen/tools/${command}"
        exit 0
    fi
    # ${pkgs[@]+…}: under set -u, bash 3 treats an empty array as unset.
    for p in ${pkgs[@]+"${pkgs[@]}"}; do
        if [ "$(name_of "$p")" = "$command" ]; then
            go -C .limen/tools get -tool "${p}@latest"   # bump .limen/tools/go.mod to the latest release
            go -C .limen/tools mod tidy
            {{ go_tool_build }} "$p"              # REAL build: a bad pin fails here, not at first use
            exit 0
        fi
    done
    if ! project_aqua update "$command"; then   # bump .hermetic/hermetic.yaml to the latest release
        echo "${command} is not pinned in .hermetic/hermetic.yaml — a canonical tool moves with limen; pin your own version with 'just do tools add <package>'." >&2
        exit 1
    fi
    {{ agent_home }} {{ aqua_api }} -c .hermetic/hermetic.yaml update-checksum --prune      # new checksum in, replaced version's out
    {{ agent_home }} hermetic -c .hermetic/hermetic.yaml install                      # REAL install: a bad pin fails here, not at first use

# Remove a tool entirely, e.g. `just do tools remove junegunn/fzf`.
[positional-arguments]
remove pkg: (_banner "tools" "remove")
    #!/usr/bin/env bash
    set -euo pipefail
    pkg="$1"
    # See `add` for the escaping.
    esc=$(printf '%s' "$pkg" | sed 's/[.^$*+?()[{}|]/\\&/g')
    first=$pkg; first=${first%%/*}
    if [[ $first == *.* ]]; then
        # The built binary, named as go names it: last element, /vN stripped.
        name=${pkg##*/}
        if [[ $name =~ ^v[0-9]+$ ]]; then name=${pkg%/*}; name=${name##*/}; fi
        # An isolated module (.limen/tools/<name>/go.mod) is the tool's alone: it
        # goes as a whole, go.sum included.
        if grep -qE "^(tool[[:space:]]+)?[[:space:]]*${esc}([[:space:]]|$)" ".limen/tools/${name}/go.mod" 2>/dev/null; then
            rm -rf ".limen/tools/${name}" "$LIMEN_BUILD/tools/${name}" "$LIMEN_BUILD/tools/${name}.exe"
            exit 0
        fi
        if ! grep -qE "^(tool[[:space:]]+)?[[:space:]]*${esc}([[:space:]]|$)" .limen/tools/go.mod 2>/dev/null; then
            echo "${pkg} is not a tool directive in .limen/tools/go.mod — nothing to remove." >&2
            exit 1
        fi
        go -C .limen/tools get -tool "${pkg}@none"   # drops the directive
        go -C .limen/tools mod tidy                     # and the requirements only it needed
        rm -f "$LIMEN_BUILD/tools/${name}" "$LIMEN_BUILD/tools/${name}.exe"
        exit 0
    fi
    if ! grep -qE "^[[:space:]]*-[[:space:]]*name:[[:space:]]*${esc}([[:space:]]|@|$)" .hermetic/hermetic.yaml; then
        echo "${pkg} is not in .hermetic/hermetic.yaml — nothing to remove." >&2
        exit 1
    fi
    # See `set` for why mktemp gets an explicit sibling-path template.
    tmp=$(mktemp .hermetic/hermetic.yaml.XXXXXX)
    trap 'rm -f "$tmp"' EXIT
    # Exact string compare on the parsed slug, not a regex (a dot would be a
    # wildcard, and escaping for awk is unportable: -v eats backslashes). The
    # entry's extent is decided by indentation, as YAML decides it: deeper lines
    # belong to it, a line at its own depth or shallower ends it — so a
    # `# --- section ---` header at item depth stays with what follows.
    awk -v pkg="$pkg" '
        function indent(l) { match(l, /^[[:blank:]]*/); return RLENGTH }
        {
            if (skip) { if ($0 !~ /^[[:blank:]]*$/ && indent($0) > depth) next; skip = 0 }
            if (match($0, /^[[:blank:]]*-[[:blank:]]*name:[[:blank:]]*/)) {
                slug = substr($0, RLENGTH + 1)
                sub(/[[:blank:]].*$/, "", slug)   # drop trailing spaces / comment
                sub(/@.*$/, "", slug)             # drop @version
                if (slug == pkg) { skip = 1; depth = indent($0); next }
            }
            print
        }
    ' .hermetic/hermetic.yaml > "$tmp"
    mv "$tmp" .hermetic/hermetic.yaml
    # No `remove`: it would uninstall from the store, which every checkout on
    # the machine shares, and another may still pin the same version (the
    # canonical set imports it, for one).
    {{ agent_home }} {{ aqua_api }} -c .hermetic/hermetic.yaml update-checksum --prune      # drop the now-unused checksum
