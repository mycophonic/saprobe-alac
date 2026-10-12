# DO NOT EDIT MANUALLY.
# This file provides shared tasks common to all projects and managed by limen.
# Project recipes live in the root .justfile, below the shared-baseline import.

# Imported flat into the canonical .justfile — not a module — so the recipe can
# take arguments (`just do release v1.2.3`; a module invocation would parse the
# tag as a recipe path). Deliberately no settings and no _lib import here: an
# import's settings would apply to the whole importing module.

# Lanes: `vX.Y.Z` cuts the signed tag and pushes (CI builds, signs keyless,
# publishes; without a .release-go.yaml, the recipe creates the release page
# itself); `--local <key> vX.Y.Z` is the fully local, key-based lane and the
# sole publisher; `--local --dry-run` is an unsigned snapshot; `--ci` is the
# workflow half. usage() below and book/recipes.md have the details.
#
# Tag pushes are exactly one tag, never --tags. A signed tag already on HEAD is
# reused, so a failed publish retries safely.
[doc('Release: `vX.Y.Z` signed tag (+ CI artifacts when .release-go.yaml exists) · `--local <cosign-key> vX.Y.Z` · `--local --dry-run` · `--ci`')]
release *args:
    #!/usr/bin/env bash
    set -euo pipefail
    # Artifacts are opt-in by carrying a goreleaser config. Without one the
    # default lane still releases: the signed tag IS the release (a Go module, a
    # tap, a configuration). --local and --ci run goreleaser, so they require it.
    has_goreleaser() { [ -f .release-go.yaml ]; }
    require_goreleaser() {
        if ! has_goreleaser; then
            echo "this project has no .release-go.yaml — this lane runs goreleaser (see book/recipes.md); a tag-only release is 'just do release vX.Y.Z'." >&2
            exit 1
        fi
    }
    usage() {
        echo "usage: just do release <tag>                                               # signed tag + push; CI builds, signs (keyless), publishes (no .release-go.yaml: the release page, with notes)" >&2
        echo "       just do release --local <cosign-key> <tag> [--cosign-password-stdin] # fully local, key-based cosign" >&2
        echo "       just do release --local --dry-run                                   # local unsigned snapshot into build/release/" >&2
        echo "       just do release --ci                                                # workflow half of the default lane (GitHub Actions only)" >&2
    }
    require_clean() {
        # A dirty tree is the most common failure and goreleaser hard-refuses
        # it — check long before anything is created or pushed.
        if [ -n "$(git status --porcelain)" ]; then
            echo "the working tree is dirty — commit or stash before releasing:" >&2
            git status --short >&2
            exit 1
        fi
    }
    # A fourth segment is a fork's rebuild of the same upstream version
    # (v10.6.1.2, book/forks.md): never a -suffix, which semver sorts before
    # the release it rebuilds.
    require_version_tag() {
        if ! printf '%s' "$1" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?(-[0-9A-Za-z][0-9A-Za-z.-]*)?$'; then
            echo "'$1' is not a version tag (vX.Y.Z, vX.Y.Z-prerelease, or a fork's rebuild vX.Y.Z.N)." >&2
            exit 2
        fi
    }
    ensure_signed_tag() {
        # Create the signed tag — unless it already exists pointing at HEAD, so
        # a retry after a failed publish resumes instead of failing here.
        if existing=$(git rev-parse -q --verify "refs/tags/$1^{commit}"); then
            if [ "$existing" != "$(git rev-parse HEAD)" ]; then
                echo "tag $1 already exists and does not point at HEAD — refusing." >&2
                exit 1
            fi
            # Only reuse a pre-existing tag if it is itself SIGNED — otherwise a
            # lightweight or unsigned tag placed on HEAD by hand would be pushed,
            # silently bypassing the `git tag -s` guarantee. We check that a
            # signature block is PRESENT (a lightweight tag has no tag object;
            # `git cat-file tag` on it fails and greps empty) rather than running
            # `git verify-tag`: verification additionally needs the signer's key
            # to be trusted — the GPG keyring, or gpg.ssh.allowedSignersFile for
            # SSH — and would reject a legitimately SSH-signed tag when that is
            # not set up, breaking the very retry this branch exists for.
            if ! git cat-file tag "refs/tags/$1" 2>/dev/null | grep -qE '^-----BEGIN [A-Z0-9]+ SIGNATURE-----'; then
                echo "tag $1 is on HEAD but is not a signed tag — refusing to reuse it." >&2
                echo "delete it and let the release create a fresh signed tag: git tag -d $1" >&2
                exit 1
            fi
            echo "tag $1 already on HEAD (signed) — reusing it."
        else
            git tag -s "$1" -m "$1"
        fi
    }
    # CI lane: pushing the v* tag is what starts the release workflow — the one
    # publisher in this lane.
    # No tag while a pull request is open, a bot's included: what is open is
    # part of the release or the next one's first commit, and both say wait
    # (book/recipes.md). gh resolves the repository from origin, the remote
    # the tag goes to.
    require_empty_queue() {
        local open
        open=$(gh pr list --state open --json number --jq 'length' -R "$(git remote get-url origin)")
        if [ "$open" != 0 ]; then
            echo "$open pull request(s) are open — a release waits for an empty queue (merge or close them first):" >&2
            gh pr list --state open -R "$(git remote get-url origin)" >&2
            exit 1
        fi
    }
    tag_and_push() {
        ensure_signed_tag "$1"
        git push origin HEAD
        git push origin "refs/tags/$1"
    }
    set -- {{ args }}
    case "${1:-}" in
        --local)
            shift
            require_goreleaser
            if [ "${1:-}" = "--dry-run" ]; then
                # Snapshot builds skip signing: no key material needed.
                exec goreleaser release --config .release-go.yaml --snapshot --clean --skip=sign
            fi
            key="${1:-}"
            if [ -z "$key" ]; then
                usage
                exit 2
            fi
            shift
            tag=""
            password=""
            password_from_stdin=""
            while [ $# -gt 0 ]; do
                case "$1" in
                    --cosign-password-stdin)
                        # A boolean flag, NOT a value: the passphrase is read from
                        # stdin below, so it never enters argv or shell history.
                        password_from_stdin=1
                        shift
                        ;;
                    -*)
                        echo "unknown option: $1" >&2
                        usage
                        exit 2
                        ;;
                    *)
                        if [ -n "$tag" ]; then
                            echo "unexpected argument: $1 (tag already given: $tag)" >&2
                            usage
                            exit 2
                        fi
                        tag="$1"
                        shift
                        ;;
                esac
            done
            if [ -z "$tag" ]; then
                usage
                exit 2
            fi
            if [ ! -f "$key" ]; then
                echo "cosign key not found: $key" >&2
                echo "generate a key pair once with 'cosign generate-key-pair'; commit cosign.pub, keep the private key out of the tree." >&2
                exit 1
            fi
            require_version_tag "$tag"
            require_clean
            require_empty_queue
            if [ -z "${GITHUB_TOKEN:-}" ]; then
                echo "GITHUB_TOKEN is not set — goreleaser needs it to publish the GitHub release." >&2
                exit 1
            fi
            # cosign under goreleaser cannot prompt, so the passphrase travels as
            # COSIGN_PASSWORD: read once here, never through argv. Empty is legal
            # (unencrypted key).
            if [ -n "$password_from_stdin" ]; then
                # --cosign-password-stdin: the passphrase is piped in, so `cat`
                # would echo it in cleartext if stdin were the terminal. Refuse
                # that footgun and point at the two safe forms.
                if [ -t 0 ]; then
                    echo "--cosign-password-stdin expects the passphrase piped on stdin, but stdin is a terminal." >&2
                    echo "pipe it (e.g. 'pass show cosign | just do release --local $key $tag --cosign-password-stdin'), or drop the flag to be prompted." >&2
                    exit 2
                fi
                # Read ALL of stdin; command substitution strips trailing
                # newlines, so a here-string, a `printf | ` pipe, and a file with
                # or without a trailing newline all yield the same passphrase.
                password=$(cat)
            else
                # No stdin channel: prompt on the terminal itself (/dev/tty, never
                # fd 0 — goreleaser may want stdin, and fd 0 may be redirected).
                printf 'cosign key password (empty for an unencrypted key): ' >&2
                read -rs password < /dev/tty
                echo >&2
            fi
            export COSIGN_KEY="$key"
            export COSIGN_PASSWORD="$password"
            # Sole publisher: push the commit, never the tag. goreleaser creates the
            # release and the remote tag through the API, and with no v* tag push
            # the release workflow never fires alongside it. The signed tag stays
            # local.
            ensure_signed_tag "$tag"
            git push origin HEAD
            exec goreleaser release --config .release-go.yaml --clean
            ;;
        --ci)
            # The workflow half: the tag exists (its push triggered this run,
            # and CI checks out exactly that commit), publishing uses the
            # workflow's GITHUB_TOKEN, and signing is keyless — the OIDC
            # identity replaces any key.
            require_goreleaser
            if [ -z "${GITHUB_ACTIONS:-}" ]; then
                echo "--ci is the workflow half — locally, 'just do release <tag>' cuts the tag CI releases." >&2
                exit 1
            fi
            if [ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" ]; then
                echo "no OIDC token available — the workflow needs 'id-token: write' for keyless signing." >&2
                exit 1
            fi
            if [ -z "${GITHUB_TOKEN:-}" ]; then
                echo "GITHUB_TOKEN is not set — pass the workflow token to this step's env." >&2
                exit 1
            fi
            if ! git describe --exact-match --tags HEAD >/dev/null 2>&1; then
                echo "HEAD carries no tag — this lane only runs on a tag push." >&2
                exit 1
            fi
            # Keyless is enforced, not defaulted: a key leaking into the CI
            # environment must not silently flip the signing mode.
            unset COSIGN_KEY
            exec goreleaser release --config .release-go.yaml --clean
            ;;
        --dry-run)
            echo "dry runs are a local concern: use 'just do release --local --dry-run'." >&2
            exit 2
            ;;
        "")
            usage
            exit 2
            ;;
        -*)
            echo "unknown option: $1" >&2
            usage
            exit 2
            ;;
        *)
            # The default lane: cut the signed tag and push — the release
            # workflow takes it from here.
            tag="$1"
            shift
            if [ $# -gt 0 ]; then
                echo "unexpected argument: $1" >&2
                usage
                exit 2
            fi
            require_version_tag "$tag"
            require_clean
            require_empty_queue
            tag_and_push "$tag"
            if has_goreleaser; then
                echo "tag $tag pushed — the release workflow builds, signs, and publishes from here."
            else
                # No artifacts to build, but every tag has a release page,
                # with GitHub's notes from the merged pull requests (grouped
                # by .github/release.yml): the laptop makes it, through the
                # pinned gh and the human's own auth, on the repository the
                # tag went to (gh would otherwise pick between origin and
                # upstream by its own rules). --verify-tag refuses a tag that
                # has not reached origin. GitHub starts the notes at the last
                # PUBLISHED release, so a repository whose earlier tags had no
                # page gets every pull request since inception unless the
                # previous tag is named. A retry finds the page and leaves it.
                repo="$(git remote get-url origin)"
                if gh release view "$tag" -R "$repo" >/dev/null 2>&1; then
                    echo "tag $tag pushed — its release page already exists (a retry): nothing to build or publish."
                else
                    opts=(--verify-tag --generate-notes)
                    # The previous tag must exist on the remote: a tag deleted
                    # there survives in a clone (pull prunes no tags), and gh
                    # answers HTTP 400 to a start tag GitHub does not know.
                    if previous="$(git describe --tags --match "v*" --abbrev=0 "$tag^" 2>/dev/null)"; then
                        if git ls-remote --exit-code --tags origin "refs/tags/$previous" >/dev/null 2>&1; then
                            opts+=(--notes-start-tag "$previous")
                        else
                            echo "previous tag $previous is not on origin (deleted there, kept here): the notes start at GitHub's last published release." >&2
                        fi
                    fi
                    case "$tag" in *-*) opts+=(--prerelease) ;; esac
                    gh release create "$tag" -R "$repo" "${opts[@]}"
                    echo "tag $tag pushed and its release page created — no .release-go.yaml, so nothing else to build or publish."
                fi
            fi
            ;;
    esac
