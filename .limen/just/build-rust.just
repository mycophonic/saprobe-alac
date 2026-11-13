# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

set working-directory := '../..'

# Silence just's per-line command echo; recipes announce themselves via _banner.
set quiet

# Shared private _banner recipe (see lib.just).
import 'lib.just'

default: release

# The crate's release binaries for this host, into target/release/ (cargo's own
# layout; the pinned toolchain through rust_env in lib.just). --locked:
# Cargo.lock is the crate pin.
release: (_banner "build rust" "release")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f Cargo.toml ]; then
        echo "no Cargo.toml — not a Rust crate, nothing to build (skipped, not failed)."
        exit 0
    fi
    {{ rust_env }}
    cargo build --release --locked

# Release binaries for other targets, each into target/<triple>/release/,
# linked through the project's pinned zig (ziglang/zig in
# .hermetic/hermetic.yaml) by the project's pinned cargo-zigbuild
# (rust-cross/cargo-zigbuild): one C toolchain for the C and the Rust builds,
# and a macOS or Windows target links from any host (zig ships the libSystem
# stubs and the MinGW-w64 CRT). cargo-zigbuild is what maps a triple to a zig
# target, hands cargo and the cc crate the linker, and drops or rewrites the
# arguments rustc means for GNU ld that zig's lld refuses, a list that moves
# with rustc and zig: a wrapper of our own would be a copy of it, behind. Each
# target's rust-std comes from rust-toolchain.toml's `targets`, fetched by
# rustup and verified like the rest. The triples reach the script as
# arguments, never interpolated into it.
[positional-arguments]
targets +triples: (_banner "build rust" "targets")
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f Cargo.toml ]; then
        echo "no Cargo.toml — not a Rust crate, nothing to build (skipped, not failed)."
        exit 0
    fi
    for tool in zig cargo-zigbuild; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            echo "$tool is not on the hermetic PATH: cross builds link through the project's pinned zig and cargo-zigbuild (ziglang/zig and rust-cross/cargo-zigbuild in .hermetic/hermetic.yaml)." >&2
            exit 1
        fi
    done
    {{ rust_env }}
    # Every cache under .build/: zig's two, and cargo-zigbuild's own (the libc
    # and SDK stubs it generates), which otherwise lands in the user's cache
    # directory.
    export ZIG_GLOBAL_CACHE_DIR="$LIMEN_BUILD/zig/global" ZIG_LOCAL_CACHE_DIR="$LIMEN_BUILD/zig/local"
    export CARGO_ZIGBUILD_CACHE_DIR="$LIMEN_BUILD/zig/cargo-zigbuild"
    # For the host's own triple, cargo-zigbuild sets target-applies-to-host to
    # false: build scripts and proc-macros stop reading [target.<host>] and
    # read [host] instead. The linker rust_env gave the host (rust-lld on the
    # arm64 Windows one, whose default is a clang it does not ship) goes to
    # [host] for that build, and leaves the target's variable: cargo-zigbuild
    # sets a target's linker only where none is set, so left exported it
    # would link the product itself, and not zig.
    host="$(rustc -vV | sed -n 's/^host: //p')"
    host_linker_var="CARGO_TARGET_$(printf '%s' "$host" | tr 'a-z-' 'A-Z_')_LINKER"
    host_linker="${!host_linker_var:-}"
    for triple in "$@"; do
        echo "→ target/$triple/release"
        if [ "$triple" = "$host" ] && [ -n "$host_linker" ]; then
            (
                unset "$host_linker_var"
                CARGO_UNSTABLE_HOST_CONFIG=true CARGO_HOST_LINKER="$host_linker" cargo zigbuild --release --locked --target "$triple"
            )
        else
            cargo zigbuild --release --locked --target "$triple"
        fi
    done
