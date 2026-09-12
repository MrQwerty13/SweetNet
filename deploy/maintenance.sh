#!/usr/bin/env bash
# Shared by backup.sh / restore.sh; Bash 3.2+ and Python 3.9+.

die() { printf '%s\n' "$1" >&2; exit 1; }

initialize() {
  REPO=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
  ENV_FILE=$1
  PROJECT=$2
  [[ -f "$ENV_FILE" ]] || die 'Explicit environment file not found.'
  ENV_FILE=$(cd -- "$(dirname -- "$ENV_FILE")" && pwd -P)/$(basename -- "$ENV_FILE")
  [[ "$PROJECT" =~ ^[a-z0-9][a-z0-9_-]{0,47}$ ]] || die 'Use a project name of 1–48 lowercase letters, digits, underscores or hyphens.'
  command -v docker >/dev/null || die 'Docker CLI is required.'
  command -v python3 >/dev/null || die 'Python 3.9+ is required.'
  python3 -c 'import sys; sys.exit(sys.version_info < (3, 9))' || die 'Python 3.9+ is required.'
  # The explicit file is authoritative; ignore exported interpolation overrides.
  unset POSTGRES_USER POSTGRES_DB POSTGRES_PASSWORD APP_ORIGIN APP_ENV APP_PORT
  unset HTTP_ADDR UPLOAD_DIR WEB_DIR DATABASE_URL SWEETNET_IMAGE
  unset COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_PROFILES COMPOSE_ENV_FILES
  unset COMPOSE_REMOVE_ORPHANS
  dc config --quiet >/dev/null 2>&1 || die 'Compose configuration is invalid; check the explicit environment file.'
  dc config --format json 2>/dev/null | python3 -c '
import json, re, sys
c = json.load(sys.stdin)
e = c["services"]["db"]["environment"]
assert all(re.fullmatch(r"[A-Za-z0-9_]+", e[k]) for k in ("POSTGRES_USER", "POSTGRES_DB"))
assert re.fullmatch(r"[A-Za-z0-9._~-]+", e["POSTGRES_PASSWORD"])
' >/dev/null 2>&1 || die 'Database names must be alphanumeric/underscore; use a URL-safe database password (for example random hex).'
  docker info >/dev/null 2>&1 || die 'Docker daemon is unavailable.'
  LOCK_NAME="sweetnet-${PROJECT}-maintenance-lock"
  LOCK_ID=
  RESUME_APP=0
  trap finish EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  trap 'exit 129' HUP
}

dc() {
  docker compose --project-directory "$REPO" --env-file "$ENV_FILE" \
    -f "$REPO/compose.yaml" -p "$PROJECT" "$@"
}

finish() {
  result=$?
  trap - EXIT INT TERM HUP
  if [[ "$RESUME_APP" == 1 ]]; then
    if ! dc start --wait --wait-timeout 120 app >/dev/null 2>&1; then
      printf '%s\n' 'ERROR: app restart/readiness failed; run Compose start app manually with the same project and environment file.' >&2
      result=1
    fi
  fi
  if [[ -n "$LOCK_ID" ]]; then
    if ! docker rm "$LOCK_ID" >/dev/null 2>&1; then
      printf '%s\n' 'ERROR: maintenance lock remains; inspect it before the next maintenance operation.' >&2
      result=1
    fi
  fi
  exit "$result"
}

acquire_lock() {
  # An unstarted, volume-free container gives an atomic lock on this Docker
  # daemon, including when scripts are invoked from different host directories.
  LOCK_ID=$(docker create --name "$LOCK_NAME" --network none \
    --label sweetnet.maintenance=true --entrypoint /bin/true "$IMAGE_ID" 2>/dev/null) \
    || die 'Cannot acquire maintenance lock; another operation may be active.'
}

assert_volume_mount() {
  docker inspect --format '{{range .Mounts}}{{println .Name .Destination}}{{end}}' "$1" 2>/dev/null \
    | python3 -c 'import sys; assert sys.argv[1] + " " + sys.argv[2] in sys.stdin.read().splitlines()' "$2" "$3" \
    >/dev/null 2>&1 || die 'Unexpected volume mapping; this script supports the supplied Compose storage layout only.'
}

volume_tool() {
  # No application/database environment, network access, or private stdout logs.
  docker run --rm -i --log-driver none --network none --read-only --user 10001:10001 \
    --cap-drop ALL --security-opt no-new-privileges:true \
    --mount "type=volume,src=${PROJECT}_uploads,dst=/data/uploads${1}" \
    --entrypoint /bin/sh "$IMAGE_ID" -c "$2"
}
