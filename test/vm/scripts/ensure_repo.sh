#!/usr/bin/env sh
set -eu

repo_input="${1:-}"
branch_input="${2:-}"

repo="${repo_input:-${STACCATO_REPO:-}}"
if [ -z "$repo" ]; then
  echo "repo is required (arg1 or STACCATO_REPO)" >&2
  exit 2
fi

branch="${branch_input:-${STACCATO_ENVIRONMENT:-}}"
if [ -z "$branch" ]; then
  branch="$(basename "$(pwd)")"
fi
if [ -z "$branch" ]; then
  echo "branch is required (arg2, STACCATO_ENVIRONMENT, or env directory name)" >&2
  exit 2
fi

case "$repo" in
  http://*|https://*|ssh://*|git@*) repo_url="$repo" ;;
  *) repo_url="https://github.com/${repo}.git" ;;
esac

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  current_origin="$(git remote get-url origin 2>/dev/null || true)"
  if [ -z "$current_origin" ]; then
    git remote add origin "$repo_url"
  elif [ "$current_origin" != "$repo_url" ]; then
    echo "origin remote mismatch: have '$current_origin', expected '$repo_url'" >&2
    exit 1
  fi
else
  if [ -n "$(find . -mindepth 1 -maxdepth 1 -not -name '.git' -print -quit)" ]; then
    echo "directory is not empty and not a git repo; refusing to initialize" >&2
    exit 1
  fi
  git init >/dev/null
  git remote add origin "$repo_url"
fi

echo "syncing $repo_url branch $branch"
git fetch --prune origin "+refs/heads/$branch:refs/remotes/origin/$branch"

if ! git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
  echo "remote branch '$branch' not found on origin" >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/heads/$branch"; then
  git checkout "$branch" >/dev/null
else
  git checkout -B "$branch" "origin/$branch" >/dev/null
fi

git reset --hard "origin/$branch" >/dev/null
git clean -fd >/dev/null

sha="$(git rev-parse --short=12 HEAD)"
echo "repo ensured: $(git remote get-url origin) @ $branch ($sha)"
