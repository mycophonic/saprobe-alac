# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

rust: (_banner "fix" "rust")
    cargo clippy --fix --all-targets --all-features --allow-dirty --allow-staged
    cargo fmt --all
