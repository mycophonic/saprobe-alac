# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: rust

# The mutating twins of `just do lint rust`, from the same pinned toolchain.
rust: (_banner "fix" "rust")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f Cargo.toml ]; then
        echo "no Cargo.toml — not a Rust crate, nothing to fix (skipped, not failed)."
        exit 0
    fi
    {{ rust_env }}
    # Under the sessions' sandbox, with the human's HOME: cargo fix discovers
    # the repository in-process (not through git, so GIT_CONFIG_GLOBAL is not
    # read) and opens the home's git configuration on the way, which the
    # sandbox denies, and the discovery fails as "no VCS found". The home
    # swap the hermetic recipes already use (agent_home) gives it a home with
    # nothing to read; CARGO_HOME and RUSTUP_HOME are explicit above, so the
    # swap changes nothing else.
    {{ agent_home }} cargo clippy --fix --all-targets --all-features --locked --allow-dirty --allow-staged
    cargo fmt --all
