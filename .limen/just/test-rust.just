# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: unit

# The crate's tests, every feature on, from the pinned toolchain (rust_env in
# lib.just). --locked: Cargo.lock is the crate pin.
unit: (_banner "test rust" "unit")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f Cargo.toml ]; then
        echo "no Cargo.toml — not a Rust crate, nothing to test (skipped, not failed)."
        exit 0
    fi
    {{ rust_env }}
    cargo test --all-features --locked
