# Apple signing, notarization and packaging, for a project that releases a
# bundle macOS evaluates (book/releasing.md, the Apple policy), under `just
# do` like release. The project keeps its build and composes its own
# `package <version>` and `verify-assets <version>` from these; the Apple
# release lane's sign job runs `package`, goreleaser's before hook runs
# `verify-assets`.
#
# The names are the lane's, exported by its sign job and read here, never the
# project's: APPLE_SIGN_IDENTITY (the identity's name, "Developer ID
# Application"), APPLE_SIGN_TEAM (the ten-character team id read off the
# certificate), APPLE_SIGNING_KEYCHAIN, APPLE_PROVISIONING_PROFILE_PATH, and
# for notarization APPLE_NOTARY_KEY (the .p8's path), APPLE_NOTARY_KEY_ID,
# APPLE_NOTARY_ISSUER_ID. Unset, every recipe treats the build as an ad hoc
# one: nothing to notarize, and an ad hoc signature accepted off the release
# tag (apple-verify-signed).

# codesign's verdict on a bundle. On a release run the signature must be the
# Developer ID Application identity's, timestamped (a signature without a
# timestamp expires with the certificate); anywhere else an ad hoc signature
# is accepted as such. A release run is the sign job, which exports the
# identity, or the publish job, on the release tag (GITHUB_REF_TYPE); the
# verify lane builds the same bundle ad hoc on branches and pull requests,
# in GitHub Actions too, so "in Actions" is not the key. Notarization is a
# separate verdict (apple-verify-notarized): the policy signs every bundle
# and notarizes the ones a human installs.
[doc('Assert a bundle is signed: Developer ID and timestamped on a release run, ad hoc accepted elsewhere')]
apple-verify-signed bundle:
    #!/usr/bin/env bash
    set -euo pipefail
    fail() { echo "apple-verify-signed: $*" >&2; exit 1; }
    bundle='{{ bundle }}'
    [ -e "$bundle" ] || fail "$bundle does not exist"
    codesign --verify --deep --strict "$bundle" || fail "the code signature of $bundle does not verify"
    details=$(codesign -d --verbose=2 "$bundle" 2>&1)
    if printf '%s\n' "$details" | grep -q '^Authority=Developer ID Application'; then
        printf '%s\n' "$details" | grep -q '^Timestamp=' || fail "the Developer ID signature of $bundle carries no timestamp"
        echo "apple-verify-signed: $bundle is signed by Developer ID Application, timestamped"
    else
        if [ -n "${APPLE_SIGN_IDENTITY:-}" ] || [ "${GITHUB_REF_TYPE:-}" = tag ]; then
            fail "$bundle is not Developer ID signed; a release run's bundle must be"
        fi
        echo "apple-verify-signed: $bundle is ad hoc signed (not a release run)"
    fi

# The stapled notarization ticket: Contents/CodeResources is where stapler
# writes it, and `stapler validate` is the check that needs no network, the
# one Gatekeeper makes offline. spctl has no assessment for a plug-in (its
# execute type answers for app bundles alone), so it is not consulted.
[doc('Assert a bundle carries a stapled notarization ticket that validates')]
apple-verify-notarized bundle:
    #!/usr/bin/env bash
    set -euo pipefail
    fail() { echo "apple-verify-notarized: $*" >&2; exit 1; }
    bundle='{{ bundle }}'
    [ -f "$bundle/Contents/CodeResources" ] || fail "no notarization ticket is stapled to $bundle"
    xcrun stapler validate "$bundle" || fail "the ticket stapled to $bundle does not validate"
    echo "apple-verify-notarized: $bundle carries a valid stapled ticket"

# Submit the signed bundle to Apple and staple the ticket. The verdict is
# notarytool's own "status: Accepted": --wait's exit status is not it. Without
# an identity (a local build) there is nothing to notarize and the recipe says
# so; with one and no credentials it fails, since a Developer ID build that
# leaves here unnotarized would be refused on every install.
[doc('Notarize a signed bundle with notarytool and staple the ticket')]
apple-notarize bundle:
    #!/usr/bin/env bash
    set -euo pipefail
    bundle='{{ bundle }}'
    if [ -z "${APPLE_SIGN_IDENTITY:-}" ]; then
        echo "apple-notarize: no signing identity, $bundle is ad hoc signed — nothing to notarize"
        exit 0
    fi
    for v in APPLE_NOTARY_KEY APPLE_NOTARY_KEY_ID APPLE_NOTARY_ISSUER_ID; do
        [ -n "${!v:-}" ] || { echo "apple-notarize: $v is not set; a Developer ID build must be notarized" >&2; exit 1; }
    done
    mkdir -p "$LIMEN_BUILD/notarize"
    archive="$LIMEN_BUILD/notarize/$(basename "$bundle").zip"
    rm -f "$archive"
    ditto -c -k --sequesterRsrc --keepParent "$bundle" "$archive"
    key=(--key "${APPLE_NOTARY_KEY:?}" --key-id "${APPLE_NOTARY_KEY_ID:?}" --issuer "${APPLE_NOTARY_ISSUER_ID:?}")
    submission=$(xcrun notarytool submit "$archive" --wait "${key[@]}" 2>&1 | tee /dev/stderr) || true
    id=$(printf '%s\n' "$submission" | sed -n 's/^ *id: \([0-9a-f-]*\)$/\1/p' | head -n 1)
    if ! printf '%s\n' "$submission" | grep -q '^ *status: Accepted$'; then
        echo "apple-notarize: Apple did not accept the submission${id:+ $id}" >&2
        [ -z "$id" ] || xcrun notarytool log "$id" "${key[@]}" >&2 || true
        exit 1
    fi
    xcrun stapler staple "$bundle"
    xcrun stapler validate "$bundle"
    echo "apple-notarize: $bundle notarized (submission ${id:-?}), ticket stapled"

# ditto, not zip: the archiver that keeps a bundle's extended attributes and
# code signature intact, so what a user unzips is what was signed.
[doc('Zip a bundle with ditto, keeping its signature intact')]
apple-package bundle zip:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "$(dirname '{{ zip }}')"
    rm -f '{{ zip }}'
    ditto -c -k --sequesterRsrc --keepParent '{{ bundle }}' '{{ zip }}'
    echo "apple-package: {{ zip }}"
