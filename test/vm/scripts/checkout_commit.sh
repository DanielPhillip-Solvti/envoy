#!/usr/bin/env sh
set -eu

commit_ref="${1:-}"
if [ -z "$commit_ref" ]; then
  echo "usage: checkout_commit.sh <commit-sha>" >&2
  exit 2
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git is required" >&2
  exit 1
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "current environment is not a git repository; run ensure_repo first" >&2
  exit 1
fi

current_branch="$(git branch --show-current 2>/dev/null || true)"
if [ -z "$current_branch" ]; then
  echo "current HEAD is detached; switch to a branch before checkout_commit" >&2
  exit 1
fi

echo "fetching latest refs for branch $current_branch"
git fetch --prune origin "+refs/heads/$current_branch:refs/remotes/origin/$current_branch"

if ! git rev-parse --verify "$commit_ref^{commit}" >/dev/null 2>&1; then
  echo "commit $commit_ref was not found after fetch" >&2
  exit 1
fi

target_ref="HEAD"
if git show-ref --verify --quiet "refs/remotes/origin/$current_branch"; then
  target_ref="origin/$current_branch"
fi

if ! git merge-base --is-ancestor "$commit_ref" "$target_ref"; then
  echo "commit $commit_ref is not on current branch $current_branch" >&2
  exit 1
fi

echo "resetting branch $current_branch to $commit_ref and removing local changes"
git reset --hard "$commit_ref" >/dev/null
git clean -fd >/dev/null

resolved="$(git rev-parse --short=12 HEAD)"
echo "Local branch <$current_branch> reset to commit <$resolved>"
