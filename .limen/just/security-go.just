# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: vuln

# Known-vulnerability scan of the module's dependency graph against the Go
# vulnerability database (fetched once, see _vulndb in lib.just), once per
# supported platform (see _per-platform in lib.just: call paths and
# dependencies differ by platform). Reports what the module reaches, not what
# it merely imports: a finding is a call path to cut or a bump to take. No
# go.mod at all: says so and passes, so `just do security` is safe in every
# repository — the guard is a recipe of its own because a dependency cannot
# short-circuit the ones after it, and the tool build would fail first.
vuln: (_banner "security go" "vuln")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f go.mod ]; then
        echo "no go.mod — not a Go module, nothing to scan (skipped, not failed)."
        exit 0
    fi
    just do security go _vuln

_vuln: (_go-tool "govulncheck" "golang.org/x/vuln/cmd/govulncheck") _vulndb (_per-platform 'build/tools/govulncheck -db "$(cat build/vulndb.url)" ./...')
