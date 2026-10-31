# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: rust

# rustfmt and clippy from the pinned toolchain (rust_env in lib.just): both
# are components rust-toolchain.toml names. --locked: Cargo.lock is the crate
# pin, and a lane never rewrites it.
rust: (_banner "lint" "rust")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f Cargo.toml ]; then
        echo "no Cargo.toml — not a Rust crate, nothing to lint (skipped, not failed)."
        exit 0
    fi
    {{ rust_env }}
    cargo fmt --all --check
    cargo clippy --all-targets --all-features --locked -- --deny warnings
