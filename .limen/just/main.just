# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# The root .justfile is the project's own: a shim that imports this file and
# then carries the project's recipes — everything shared lives here, so the
# shim never has to change.

# The project may redefine the shared recipes below (info, or an explicit
# default) in its root .justfile: the later definition — the project's — wins.
set allow-duplicate-recipes
# Likewise for variables: a project's root .justfile may re-export any variable
# set here (notably a hermetic GO* default below) and the importing file wins —
# so an override is explicit and tracked in the .justfile, never an ambient env
# var leaking in.
set allow-duplicate-variables

# Project name, derived from the directory the root .justfile lives in.
project := file_name(justfile_directory())

# brew is captured from the ambient PATH, above the hermetic override: not
# pinnable, and installed at machine-chosen prefixes. Empty when absent (the
# homebrew recipes then refuse); override by exporting BREW_BIN.
export BREW_BIN := env_var_or_default('BREW_BIN', `command -v brew || true`)

# gpg likewise, for git to check a PGP-signed commit (GitHub signs its merge
# and web-UI commits that way): macOS has none in the base system. Empty when
# absent; Windows' ambient PATH already carries Git for Windows' own.
export GPG_BIN := if os() == 'windows' { '' } else { env_var_or_default('GPG_BIN', `command -v gpg || true`) }

# The pinned tools + base system only, so an unpinned tool fails loudly. The
# pins are hermetic's: its root's bin holds one link per tool, each a name for
# the binary, which resolves the name from this repository's .hermetic/ pins.
# The root is HERMETIC_ROOT_DIR (a session's own, from its settings) or the
# binary's default for the platform, the one limen-install fills. Windows
# (git-bash) has no knowable base-system list and uses ';': pins are
# prepended to the ambient PATH there instead.
hermetic_root := env_var_or_default('HERMETIC_ROOT_DIR', if os() == 'macos' { home_directory() / 'Library/Application Support/hermetic' } else if os() == 'windows' { env_var_or_default('LOCALAPPDATA', home_directory() / 'AppData/Local') / 'hermetic' } else { env_var_or_default('XDG_DATA_HOME', home_directory() / '.local/share') / 'hermetic' })
hermetic_bin := hermetic_root / 'bin'
export PATH := if os() == 'windows' { hermetic_bin + ';' + env_var('PATH') } else { hermetic_bin + ":/usr/bin:/bin:/usr/sbin:/sbin" }

# A tool's link is its entry point: the shell looks the name up in the root's
# bin and fails before hermetic runs when none is there, and a fresh root, or
# a tool a merged pull request added, has no link until an install plants one.
# So every recipe run plants the missing links first (link-only: no download,
# no policy, milliseconds, and no network once the root holds the registries),
# the step CI's setup-hermetic action takes before the recipes. The binary by real path, since a top-level
# expression does not see the PATH exported above. Skipped where the pins or
# the binary are absent (a repository not yet converged, a machine without
# limen-install), where the first tool fails loudly as before; not evaluated
# for --list. A failure is printed and swallowed here, so the recipe fails on
# the tool it needs rather than every recipe in a variable.
hermetic_pins := replace(justfile_directory(), '\', '/') / '.hermetic/hermetic.yaml'
hermetic_exe := if os() == 'windows' { hermetic_bin / 'hermetic.exe' } else { hermetic_bin / 'hermetic' }
_hermetic_links := if path_exists(hermetic_pins) != 'true' { '' } else if path_exists(hermetic_exe) != 'true' { '' } else { shell('"$1" --log-level warn -c "$2" install -l >&2 || true', hermetic_exe, hermetic_pins) }

# GO* variables tunnel through the pinned PATH, so each is emptied or pinned
# (go treats '' as unset; `export` propagates into module recipes, `unexport`
# does not). A project's root .justfile may override any of them, tracked.
#
#   GOROOT       an inherited value (IDEs inject one, often into the module
#                cache that `go clean -modcache` deletes) overrides where the
#                pinned go finds its stdlib; modern go derives it from its own
#                location, so '' is correct.
#   GOTOOLCHAIN  =local forbids silent toolchain switching: when go.mod
#                outpaces the pin, recipes fail loudly asking for a pin bump
#                instead of downloading an unpinned toolchain behind your back.
#   GOFLAGS      ambient flags (-mod=mod, -tags=…, -count=1) rewrite what every
#                go command does; '' leaves the recipes' own args the whole story.
#   GOSUMDB      pinned to the real checksum database so an ambient GOSUMDB=off
#                cannot defeat the GOSUMDB verification the .limen/tools/go.mod pins
#                depend on.
#   GOPRIVATE    '' so no module path is silently exempted from the proxy and
#                checksum db — a non-empty value is precisely how GOSUMDB gets
#                bypassed per-path.
#   GOPROXY      pinned to the default so an ambient GOPROXY=off or a stale
#                mirror cannot change where modules resolve from; a project
#                needing an internal proxy sets it in its own .justfile.
#   GOOS/GOARCH  '' = native. Ambient values silently cross-compile every
#                build/lint/test; the per-platform analysis sets GOOS/GOARCH
#                itself where it needs to (see _per-platform in lib.just),
#                overriding this.
#
# GOWORK is deliberately not neutralized: a go.work in the tree puts the
# recipes into workspace mode, and a build there may differ from CI
# (book/recipes.md).
export GOROOT := ''
export GOTOOLCHAIN := 'local'
export GOFLAGS := ''
export GOSUMDB := 'sum.golang.org'
export GOPRIVATE := ''
export GOPROXY := 'https://proxy.golang.org,direct'
export GOOS := ''
export GOARCH := ''

# git's transport is the configured one: a sandbox exports a GIT_SSH_COMMAND
# that outranks the tracked core.sshCommand, and every git a recipe spawns
# (the `go mod download` of a private module) would take it and fail where
# the configured transport works. The variable is set to that configuration
# where there is one, left as it is where there is none (a sandbox's proxy is
# then the only route), and plain ssh last. Not `unexport` (it stops at the
# module boundary) and not '' (git takes an empty command and cannot fork it).
export GIT_SSH_COMMAND := `git config --get core.sshCommand 2>/dev/null || printf '%s' "${GIT_SSH_COMMAND:-${GIT_SSH:-ssh}}"`

# cgo on macOS compiles with the selected developer directory's own clang and
# SDK (Xcode's toolchain, or the Command Line Tools'), never /usr/bin/clang:
# that one is a shim that runs xcrun, whose cache lives in the per-user temp
# directory, which a sandbox may deny — the build then dies before its first
# C line. Where no developer directory is selected the three stay as the
# environment has them, and go falls back to its own default. Other platforms
# are untouched; a project overrides any of them in its root .justfile.
xcode_dev := if os() == 'macos' { `xcode-select -p 2>/dev/null || true` } else { '' }
xcode_bin := if xcode_dev == '' { '' } else if path_exists(xcode_dev / 'Toolchains/XcodeDefault.xctoolchain/usr/bin/clang') == 'true' { xcode_dev / 'Toolchains/XcodeDefault.xctoolchain/usr/bin' } else if path_exists(xcode_dev / 'usr/bin/clang') == 'true' { xcode_dev / 'usr/bin' } else { '' }
xcode_sdk := if xcode_dev == '' { '' } else if path_exists(xcode_dev / 'Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk') == 'true' { xcode_dev / 'Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk' } else if path_exists(xcode_dev / 'SDKs/MacOSX.sdk') == 'true' { xcode_dev / 'SDKs/MacOSX.sdk' } else { '' }
export CC := if xcode_bin == '' { env('CC', '') } else { xcode_bin / 'clang' }
export CXX := if xcode_bin == '' { env('CXX', '') } else { xcode_bin / 'clang++' }
export SDKROOT := if xcode_sdk == '' { env('SDKROOT', '') } else { xcode_sdk }

# The checkout's output directory, gitignored: what a recipe writes and keeps
# between runs goes under it, in a subdirectory named for what it holds. The
# only spelling of the name: the shared recipes and the project's own use
# {{ LIMEN_BUILD }} or $LIMEN_BUILD, a script a recipe runs reads it from the
# environment, and CI's setup-hermetic and windows-cache-image actions export the
# same path for the steps after them. Absolute, so a script that changes
# directory still finds it; forward slashes on windows, which git-bash's own
# tools and the native ones both read.
export LIMEN_BUILD := replace(justfile_directory(), '\', '/') / '.build'

# Hermetic, per-project linter cache (same doctrine as PATH and GOROOT: no
# shared mutable state across repos). Lives under LIMEN_BUILD, so local runs
# behave like CI instead of diverging on cache state.
export GOLANGCI_LINT_CACHE := LIMEN_BUILD / 'cache/golangci-lint'

# just's default recipe is the first one defined, so a project's first own
# recipe replaces this one.
# Show every available recipe — the project's own and the shared `do` tree.
default:
    @just --list

# Print meaningful information about this project.
info:
    @echo "name:     {{ project }}"
    @echo "upstream: $(git remote get-url origin 2>/dev/null || echo '(none)')"
    @echo "semver:   $(git describe --tags --abbrev=0 2>/dev/null || echo '(none)')"
    @echo "commit:   $(git rev-parse --short HEAD 2>/dev/null || echo '(none)')"
    @echo "date:     $(git log --max-count=1 --format=%cd --date=short 2>/dev/null || echo '(none)')"

# Every shared limen task: `just do lint`, `just do build go`, `just do release v1.2.3` — the top level stays the project's.
mod do 'do.just'
