# This file is the project's own.
# Add recipes leveraging provided `do` ready-made recipes, or create your own.
# The import must be kept: it mounts every shared limen task under `just do ...`.
import '.limen/just/main.just'

# The FIRST recipe defined here becomes `just`'s default.
# The shared default covers the language-agnostic linters (limen, just, aqua,
# yaml, shell, dockerfile, commits); the Go linters (golangci-lint, govulncheck,
# go mod tidy, go-licenses) live in their own submodule and are added explicitly.
[doc('Lint everything (shared linters + Go linters)')]
lint: do::lint::default do::lint::go::default

# Shared fixers plus the Go fixers (golangci-lint --fix, go mod tidy).
[doc('Auto-fix what can be fixed (shared fixers + Go fixers)')]
fix: do::fix::default do::fix::go::default

# Unit, race and cover. Benchmarks are a performance measure, not a test:
# `just do perf go bench`. The conformance and benchmark tests skip when the
# reference tools below are not built.
[doc('Run the Go test suite: unit, race, cover')]
test: do::test::go::unit do::test::go::race do::test::go::cover

# The example decoder under cmd/, built like any Go binary: under build/.
[doc('Build the example decoder')]
build: do::build::go::default

# The shared security scans (govulncheck, for a Go module); the security
# workflow runs this recipe.
[doc('Run the security scans')]
security: do::security::default

# --- Reference tools for the conformance and benchmark tests (macOS) ---------
# Both land under bin/ at the module root, the directory agar.LookFor searches
# before PATH (it walks up from the test file to go.mod and looks in bin/
# beside it); .gitignore ignores bin. Neither is needed for the unit tests,
# which skip the legs that want them.

# Apple's open-source ALAC converter, built from the upstream repository at a
# fixed commit: the reference the conformance tests compare the decoder to.
alac_commit := 'c38887c5c5e64a4b31108733bd79ca9b2496d987'

[doc("Build Apple's reference ALAC converter under bin/")]
alacconvert:
    #!/usr/bin/env bash
    set -euo pipefail
    bin="$PWD/bin"
    src="$bin/tmp/alac"
    rm -rf "$src"
    mkdir -p "$bin"
    git clone --quiet https://github.com/macosforge/alac.git "$src"
    git -C "$src" checkout --quiet '{{ alac_commit }}'
    make -C "$src/convert-utility"
    cp "$src/convert-utility/alacconvert" "$bin/alacconvert"
    echo "alacconvert built: $bin/alacconvert"

# The CoreAudio ALAC encoder/decoder (tests/testutil/coreaudio, C): what the
# synthetic fixtures are encoded with.
[doc('Build the CoreAudio ALAC encoder/decoder under bin/')]
alac-coreaudio:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin
    make -C tests/testutil/coreaudio OUTPUT_DIR="$PWD/bin"
    echo "alac-coreaudio built: $PWD/bin/alac-coreaudio"
