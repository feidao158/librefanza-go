#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
managed_env=.env.librefanza-managed
if [ ! -f "$managed_env" ]; then
  printf '%s\n' '未找到内置服务配置' >&2
  exit 1
fi
if [ -e .env ]; then
  exec docker compose --env-file .env --env-file "$managed_env" -f compose.yaml -f compose.managed.yaml --profile postgres stop "$@"
fi
exec docker compose --env-file "$managed_env" -f compose.yaml -f compose.managed.yaml --profile postgres stop "$@"
