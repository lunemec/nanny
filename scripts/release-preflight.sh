#!/bin/sh
set -eu

fail() {
	printf 'release preflight failed: %s\n' "$1" >&2
	exit 1
}

[ -z "$(git status --porcelain)" ] || fail "checkout is dirty"

tags=$(git tag --points-at HEAD)
tag_count=$(printf '%s\n' "$tags" | awk 'NF { count++ } END { print count + 0 }')
[ "$tag_count" -eq 1 ] || fail "HEAD must have exactly one tag"
tag=$tags
printf '%s\n' "$tag" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || fail "tag must be an unprefixed semantic version"

git fetch --quiet origin '+refs/heads/master:refs/remotes/origin/master' --tags || fail "unable to refresh origin/master and tags"

head_commit=$(git rev-parse HEAD)
master_commit=$(git rev-parse refs/remotes/origin/master)
tag_commit=$(git rev-parse "$tag^{commit}")
remote_tag_commit=$(git ls-remote origin "refs/tags/${tag}^{}" | awk 'NR == 1 { print $1 }')
if [ -z "$remote_tag_commit" ]; then
	remote_tag_commit=$(git ls-remote origin "refs/tags/$tag" | awk 'NR == 1 { print $1 }')
fi

[ -n "$remote_tag_commit" ] || fail "tag $tag is not pushed to origin"
[ "$head_commit" = "$master_commit" ] || fail "HEAD does not match origin/master"
[ "$head_commit" = "$tag_commit" ] || fail "local tag $tag does not match HEAD"
[ "$head_commit" = "$remote_tag_commit" ] || fail "remote tag $tag does not match HEAD"

printf 'release preflight passed for tag %s\n' "$tag"
