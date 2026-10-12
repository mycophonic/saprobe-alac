# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

# Go tests live in their own submodule: `just do test go` runs the unit tests,
# `just do test go <recipe>` (e.g. `just do test go race`) runs one task.
#
# Bare `just do test` refuses: tests are language-bound, so any universal
# default would fail or lie somewhere. Name a suite, and define the project
# aggregate as `test` in the root .justfile (what CI runs).
default:
    @echo 'just test has no default — name a suite (e.g. `just do test go`), or run the project'"'"'s `just test`.' >&2
    @exit 1

mod go 'test-go.just'
