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

# aqua-pinned tools + base system only, so an unpinned tool fails loudly.
# Windows (git-bash) has no knowable base-system list and uses ';': pins are
# prepended to the ambient PATH there instead.
aqua_bin := env_var_or_default('AQUA_ROOT_DIR', env_var_or_default('XDG_DATA_HOME', home_directory() / '.local/share') / 'aquaproj-aqua') / 'bin'
export PATH := if os() == 'windows' { aqua_bin + ';' + env_var('PATH') } else { aqua_bin + ":/usr/bin:/bin:/usr/sbin:/sbin" }

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
#                cannot defeat the GOSUMDB verification the tools/go.mod pins
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

# Hermetic, per-project linter cache (same doctrine as PATH and GOROOT: no
# shared mutable state across repos). Lives under build/ — gitignored, and
# local runs behave like CI instead of diverging on cache state.
export GOLANGCI_LINT_CACHE := justfile_directory() / 'build/cache/golangci-lint'

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
