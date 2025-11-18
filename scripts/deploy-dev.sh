#!/usr/bin/env bash
set -euo pipefail

REMOTE="origin"
TAG_PREFIX_DEV="dev-"

usage() {
	cat <<EOF
Usage: $0 (-b <feature_branch> | -c|--current)

  -b <branch>    feature/bugfix branch (remote must exist)
  -c, --current  use current local branch as feature branch
EOF
}

FEATURE_BRANCH=""
USE_CURRENT=0

while (("$#")); do
	case "$1" in
	-b)
		FEATURE_BRANCH="$2"
		shift 2
		;;
	-c | --current)
		USE_CURRENT=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown arg: $1"
		usage
		exit 1
		;;
	esac
done

if [[ -n "$FEATURE_BRANCH" && $USE_CURRENT -eq 1 ]]; then
	echo "ERROR: use either -b or -c/--current, not both"
	exit 1
fi

if [[ -z "$FEATURE_BRANCH" && $USE_CURRENT -eq 0 ]]; then
	echo "ERROR: specify -b <branch> or -c/--current"
	usage
	exit 1
fi

if [[ $USE_CURRENT -eq 1 ]]; then
	FEATURE_BRANCH=$(git rev-parse --abbrev-ref HEAD)
fi

run() {
	echo "+ $*"
	eval "$@"
}

ensure_clean() {
	if [[ -n "$(git status --porcelain)" ]]; then
		echo "ERROR: working tree not clean"
		exit 1
	fi
}

branch_exists_remote() {
	git ls-remote --heads "$REMOTE" "$1" >/dev/null 2>&1
}

compute_dev_branch_from_remote() {
	local ref
	ref=$(git ls-remote --heads "$REMOTE" 'dev-[0-9][0-9]-[0-9][0-9]' |
		awk '{print $2}' |
		sed 's#refs/heads/##' |
		head -n1)
	if [[ -z "$ref" ]]; then
		echo "ERROR: no remote dev-??-?? branch found on ${REMOTE}" >&2
		exit 1
	fi
	echo "$ref"
}

ensure_dev_branch_present() {
	local dev="$1"
	if branch_exists_remote "$dev"; then
		run "git fetch ${REMOTE} ${dev}:${dev} || true"
	else
		echo "ERROR: remote branch ${dev} not found on ${REMOTE}"
		exit 1
	fi
}

pick_base_from_tags() {
	local dev_last
	dev_last=$(git tag -l "${TAG_PREFIX_DEV}*" --sort=-v:refname | head -n1 | sed "s/^${TAG_PREFIX_DEV}//" || true)
	if [[ -z "$dev_last" ]]; then
		echo "0.0.0"
		return 0
	fi
	echo "$dev_last" | sed -E 's/-[0-9]+$//'
}

next_dev_tag_for_base() {
	local base="$1" maxn=0 n line esc_base
	esc_base=$(printf '%s' "$base" | sed 's/\./\\./g')
	while IFS= read -r line; do
		n=$(echo "$line" | sed -nE "s/^${TAG_PREFIX_DEV}${esc_base}-([0-9]+)$/\1/p")
		[[ -n "$n" && $n -gt $maxn ]] && maxn="$n"
	done < <(git tag -l "${TAG_PREFIX_DEV}${base}-*" || true)
	echo "${TAG_PREFIX_DEV}${base}-$((maxn + 1))"
}

ensure_clean
run "git fetch --all --tags"

DEV_BRANCH="$(compute_dev_branch_from_remote)"

echo "feature: ${FEATURE_BRANCH}"
echo "dev:     ${DEV_BRANCH}"

branch_exists_remote "$FEATURE_BRANCH" || {
	echo "ERROR: remote branch ${FEATURE_BRANCH} not found"
	exit 1
}
ensure_dev_branch_present "$DEV_BRANCH"

run "git switch ${DEV_BRANCH}"
run "git pull --ff-only ${REMOTE} ${DEV_BRANCH}"

run "git merge --no-ff --no-edit ${FEATURE_BRANCH}"
run "git push ${REMOTE} ${DEV_BRANCH}"

if ! git merge-base --is-ancestor "${FEATURE_BRANCH}" "${DEV_BRANCH}"; then
	echo "ERROR: ${FEATURE_BRANCH} is not ancestor of ${DEV_BRANCH}"
	exit 1
fi

base_ver="$(pick_base_from_tags)"
tag="$(next_dev_tag_for_base "$base_ver")"
run "git tag -a \"${tag}\" -m \"Deploy to dev: ${FEATURE_BRANCH} -> ${DEV_BRANCH} (base ${base_ver})\""
run "git push ${REMOTE} \"${tag}\""

echo "created tag ${tag}"
echo "done"
