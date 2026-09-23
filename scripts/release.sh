#!/bin/sh
set -eu

fail() {
	printf 'release failed: %s\n' "$1" >&2
	exit 1
}

[ "$#" -gt 0 ] || fail "GoReleaser command is missing"

tag=${RELEASE_TAG:-}
if [ -z "$tag" ]; then
	tags=$(git tag --points-at HEAD)
	tag_count=$(printf '%s\n' "$tags" | awk 'NF { count++ } END { print count + 0 }')
	case "$tag_count" in
		0)
			[ -t 0 ] || fail "set RELEASE_TAG, for example: make release RELEASE_TAG=0.5.0"
			printf 'Release version (for example 0.5.0): ' >&2
			IFS= read -r tag || fail "no release version entered"
			;;
		1) tag=$tags ;;
		*) fail "HEAD has multiple tags; set RELEASE_TAG explicitly" ;;
	esac
fi

./scripts/release-preflight.sh --prepare "$tag"
printf 'Verifying release candidate %s before creating its tag\n' "$tag"
make release-verify VERSION="$tag"
./scripts/release-preflight.sh --prepare "$tag"

if ! git show-ref --verify --quiet "refs/tags/$tag"; then
	git tag -a "$tag" -m "Release $tag"
fi

remote_refs=$(git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}") || fail "unable to check remote tag $tag"
if [ -z "$remote_refs" ]; then
	git push origin "refs/tags/$tag"
fi

./scripts/release-preflight.sh
"$@" release --clean
