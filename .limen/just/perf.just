# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

# Reports that judge nothing: what the compiler could not eliminate, what it
# moved to the heap, how fast a function runs, where the time goes. None of
# them fails and none of them asserts, which is why they are neither lint
# (a verdict on the tree) nor test (a verdict on behavior): a number to read.
#
# Go reports live in their own submodule: `just do perf go` runs the quick
# compile-only ones, `just do perf go <recipe>` (bench, profile) runs one.
#
# Bare `just do perf` refuses, like `just do test`: every report is
# language-bound, so any universal default would fail or lie somewhere.
default:
    @echo 'just do perf has no default — name a language (e.g. `just do perf go`).' >&2
    @exit 1

mod go 'perf-go.just'
