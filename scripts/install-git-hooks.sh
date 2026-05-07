#!/bin/sh
set -eu

repo_root=$(git rev-parse --show-toplevel)
hooks_dir="$repo_root/.git/hooks"

cp "$repo_root/scripts/git-hooks/pre-commit" "$hooks_dir/pre-commit"
chmod +x "$hooks_dir/pre-commit"

echo "Installed pre-commit hook."
