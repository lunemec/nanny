#!/bin/sh
set -eu

test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM
source_root=$(git rev-parse --show-toplevel)
preflight="$source_root/scripts/release-preflight.sh"

create_repo() {
	case_name=$1
	bare_dir="$test_root/$case_name-origin.git"
	work_dir="$test_root/$case_name"
	git init --quiet --bare --initial-branch=master "$bare_dir"
	git init --quiet --initial-branch=master "$work_dir"
	(
		cd "$work_dir"
		git config user.email release-test@example.com
		git config user.name "Release Test"
		printf 'nanny\n' > README.md
		git add README.md
		git commit --quiet -m initial
		git remote add origin "$bare_dir"
		git push --quiet --set-upstream origin master
	)
}

expect_pass() {
	case_name=$1
	case_dir=$2
	shift 2
	if ! (cd "$case_dir" && "$preflight" "$@" >/dev/null); then
		printf 'expected %s to pass\n' "$case_name" >&2
		exit 1
	fi
}

expect_fail() {
	case_name=$1
	case_dir=$2
	shift 2
	if (cd "$case_dir" && "$preflight" "$@" >/dev/null 2>&1); then
		printf 'expected %s to fail\n' "$case_name" >&2
		exit 1
	fi
}

create_repo candidate
expect_pass "clean untagged candidate" "$work_dir" --prepare 0.5.0
expect_fail "invalid candidate version" "$work_dir" --prepare v0.5.0

create_repo clean
(cd "$work_dir" && git tag 0.5.0 && git push --quiet origin 0.5.0)
expect_pass "clean pushed tag" "$work_dir"
expect_pass "pushed tag retry" "$work_dir" --prepare 0.5.0

create_repo dirty
(cd "$work_dir" && git tag 0.5.0 && git push --quiet origin 0.5.0 && printf 'dirty\n' >> README.md)
expect_fail "dirty checkout" "$work_dir"
expect_fail "dirty candidate" "$work_dir" --prepare 0.5.0

create_repo unpushed
(cd "$work_dir" && git tag 0.5.0)
expect_fail "unpushed tag" "$work_dir"
expect_pass "unpushed tag retry" "$work_dir" --prepare 0.5.0

create_repo non_master
(
	cd "$work_dir"
	git checkout --quiet -b release-candidate
	printf 'candidate\n' >> README.md
	git commit --quiet -am candidate
	git tag 0.5.0
	git push --quiet origin 0.5.0
)
expect_fail "non-master commit" "$work_dir"
expect_fail "non-master candidate" "$work_dir" --prepare 0.5.0

create_repo mismatched_tag
(
	cd "$work_dir"
	git tag 0.5.0
	git push --quiet origin 0.5.0
	printf 'new master\n' >> README.md
	git commit --quiet -am "new master"
	git push --quiet origin master
	git tag --delete 0.5.0 >/dev/null
	git tag 0.5.0
)
expect_fail "mismatched remote tag" "$work_dir"
expect_fail "mismatched candidate tag" "$work_dir" --prepare 0.5.0

create_repo wrong_branch
(cd "$work_dir" && git checkout --quiet -b release-candidate)
expect_fail "candidate on branch other than master" "$work_dir" --prepare 0.5.0

create_repo other_tag
(cd "$work_dir" && git tag 0.4.5)
expect_fail "candidate with another tag at HEAD" "$work_dir" --prepare 0.5.0

create_repo prefixed
(cd "$work_dir" && git tag v0.5.0 && git push --quiet origin v0.5.0)
expect_fail "prefixed tag" "$work_dir"

create_repo release_flow
(
	cd "$work_dir"
	mkdir scripts
	cp "$source_root/scripts/release-preflight.sh" "$source_root/scripts/release.sh" scripts/
	printf 'release-verify:\n\t@false\n' > Makefile
	git add Makefile scripts
	git commit --quiet -m 'add release flow'
	git push --quiet origin master
	if RELEASE_TAG=0.5.0 ./scripts/release.sh true >/dev/null 2>&1; then
		printf 'expected failed verification to stop release\n' >&2
		exit 1
	fi
	if git show-ref --verify --quiet refs/tags/0.5.0; then
		printf 'failed verification created a tag\n' >&2
		exit 1
	fi
	printf 'release-verify:\n\t@true\n' > Makefile
	git add Makefile
	git commit --quiet -m 'pass release gates'
	git push --quiet origin master
	if RELEASE_TAG=0.5.0 ./scripts/release.sh false >/dev/null 2>&1; then
		printf 'expected failed publication to stop release\n' >&2
		exit 1
	fi
	"$preflight" >/dev/null
	./scripts/release.sh true >/dev/null
)

printf 'release preflight tests passed\n'
