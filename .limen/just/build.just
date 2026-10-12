# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Go builds live in their own submodule: `just do build go` makes the release
# binaries, `just do build go <recipe>` (debug, race, static) makes a variant.
mod go 'build-go.just'

# A submodule's recipe IS addressable as a dependency (go::default); only a
# bare module path (go) is not.
default: go::default
