#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/rubber-duck-coder/lumalabs-eng-take-home-ninad.git}"
BRANCH="${BRANCH:-main}"
DEPLOY_PATH="${DEPLOY_PATH:-$HOME/lumalabs-eng-take-home-ninad}"

if ! command -v git >/dev/null 2>&1; then
  echo "git is required on the VM"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required on the VM"
  exit 1
fi

if [ ! -d "$DEPLOY_PATH/.git" ]; then
  mkdir -p "$(dirname "$DEPLOY_PATH")"
  git clone "$REPO_URL" "$DEPLOY_PATH"
fi

cd "$DEPLOY_PATH"
git fetch origin "$BRANCH"
git pull --ff-only origin "$BRANCH"

if [ ! -f .env ]; then
  cp .env.example .env
fi

if ! grep -q '^WEB_PORT=' .env; then
  echo 'WEB_PORT=80' >> .env
fi
if ! grep -q '^API_PORT=' .env; then
  echo 'API_PORT=8080' >> .env
fi

docker compose up --build -d --remove-orphans
docker compose ps
