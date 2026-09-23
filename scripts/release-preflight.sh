#!/bin/sh
set -eu

fail() {
	printf 'release preflight failed: %s\n' "$1" >&2
	exit 1
}

case "${1:-}" in
	'') [ "$#" -eq 0 ] || fail "unexpected arguments"; mode=published ;;
	--prepare) [ "$#" -eq 2 ] || fail "usage: release-preflight.sh --prepare VERSION"; mode=prepare ;;
	*) fail "usage: release-preflight.sh [--prepare VERSION]" ;;
esac

[ -z "$(git status --porcelain)" ] || fail "checkout is dirty"
[ "$(git branch --show-current)" = master ] || fail "checkout must be on master"

git fetch --quiet origin '+refs/heads/master:refs/remotes/origin/master' --tags || fail "unable to refresh origin/master and tags"

head_commit=$(git rev-parse HEAD)
master_commit=$(git rev-parse refs/remotes/origin/master)
[ "$head_commit" = "$master_commit" ] || fail "HEAD does not match origin/master"

tags=$(git tag --points-at HEAD)
if [ "$mode" = prepare ]; then
	tag=$2
else
	tag_count=$(printf '%s\n' "$tags" | awk 'NF { count++ } END { print count + 0 }')
	[ "$tag_count" -eq 1 ] || fail "HEAD must have exactly one tag"
	tag=$tags
fi
printf '%s\n' "$tag" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || fail "tag must be an unprefixed semantic version"
[ -z "$tags" ] || [ "$tags" = "$tag" ] || fail "HEAD has a different or multiple tags"

if git show-ref --verify --quiet "refs/tags/$tag"; then
	tag_commit=$(git rev-parse "$tag^{commit}")
	[ "$head_commit" = "$tag_commit" ] || fail "local tag $tag does not match HEAD"
else
	[ "$mode" = prepare ] || fail "local tag $tag is missing"
fi

remote_refs=$(git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}") || fail "unable to check remote tag $tag"
remote_tag_commit=$(printf '%s\n' "$remote_refs" | awk '$2 ~ /\^\{\}$/ { print $1; exit }')
if [ -z "$remote_tag_commit" ]; then
	remote_tag_commit=$(printf '%s\n' "$remote_refs" | awk 'NF { print $1; exit }')
fi

if [ -n "$remote_tag_commit" ]; then
	[ "$head_commit" = "$remote_tag_commit" ] || fail "remote tag $tag does not match HEAD"
else
	[ "$mode" = prepare ] || fail "tag $tag is not pushed to origin"
fi

printf 'release preflight passed for %s\n' "$tag"
