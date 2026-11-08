#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
umask 077
managed_env=.env.librefanza-managed
if [ ! -e "$managed_env" ]; then
  random_secret() { od -An -N32 -tx1 /dev/urandom | tr -d ' \n'; }
  bind_address=${LIBREFANZA_BIND_ADDRESS:-127.0.0.1}
  case "$bind_address" in *[!0-9a-fA-F:.]*|'') printf '%s\n' '监听地址无效' >&2; exit 1;; esac
  if [ ! -e .env ] && [ -z "${DB_PASSWORD:-}" ]; then
    printf 'DB_PASSWORD=%s\n' "$(random_secret)" > "$managed_env"
  else
    : > "$managed_env"
  fi
  printf 'GORSE_API_KEY=%s\n' "$(random_secret)" >> "$managed_env"
  printf 'LIBREFANZA_BIND_ADDRESS=%s\n' "$bind_address" >> "$managed_env"
fi
chmod 600 "$managed_env"
if [ -e .env ]; then
  exec docker compose --env-file .env --env-file "$managed_env" -f compose.yaml -f compose.managed.yaml --profile postgres up -d --build "$@"
fi
exec docker compose --env-file "$managed_env" -f compose.yaml -f compose.managed.yaml --profile postgres up -d --build "$@"
