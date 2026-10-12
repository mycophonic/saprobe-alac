# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: limen just yaml aqua

# Repairs what `just do lint limen` reports; regenerates .aqua/aqua-checksums.json
# when pins changed (network). Same LIMEN_BIN override as lint.just.
limen: (_banner "fix" "limen")
    ${LIMEN_BIN:-limen} fix .

# Repair what `just do lint github` reports — plans first, applies on consent
# (pass -yes for unattended use, -org <name> for the organization). Never in
# the default set, same reasoning as the lint twin.
github *args: (_banner "fix" "github")
    ${LIMEN_BIN:-limen} github fix {{ args }}

just: (_banner "fix" "just")
    #!/usr/bin/env bash
    set -euo pipefail
    # Same enumeration and pathspecs as `lint just`, through a file likewise.
    list=$(mktemp "${TMPDIR:-/tmp}/fix-just.XXXXXX")
    trap 'rm -f "$list"' EXIT
    git ls-files -z --cached --others --exclude-standard \
        'justfile' '**/justfile' 'Justfile' '**/Justfile' '.justfile' '**/.justfile' '*.just' > "$list"
    while IFS= read -r -d '' f; do
        [ -f "$f" ] || continue
        just --fmt --justfile "$f"
    done < "$list"

yaml: (_banner "fix" "yaml")
    yamlfmt -conf .limen/.yamlfmt

# Regenerate .aqua/aqua-checksums.json from .aqua/aqua.yaml — the mutating twin of
# `just do lint aqua` (needs network). In the default set so a plain
# `just do fix` heals the drift the lint default flags.
aqua: (_banner "fix" "aqua")
    aqua --log-level warn -c .aqua/aqua.yaml update-checksum --prune

# Go fixers live in their own submodule: `just do fix go` runs them all,
# `just do fix go <recipe>` (e.g. `just do fix go mod`) runs one.
mod go 'fix-go.just'
mod rust 'fix-rust.just'
mod homebrew 'fix-homebrew.just'
