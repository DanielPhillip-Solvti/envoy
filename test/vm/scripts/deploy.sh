#!/usr/bin/env sh
set -eu
branch="${1:-main}"
mkdir -p "test/vm/envs/${branch}"
echo "deployed ${branch}"
