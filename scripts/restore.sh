#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
[[ $# == 4 && "$4" == --new-environment ]] || {
  printf '%s\n' 'Usage: scripts/restore.sh ENV_FILE NEW_PROJECT BACKUP_DIRECTORY --new-environment' >&2
  exit 2
}
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../deploy" && pwd -P)/maintenance.sh"
initialize "$1" "$2"
SOURCE=$3
APP_IMAGE=$(dc config --images app 2>/dev/null) || die 'Cannot resolve target app image.'
IMAGE_ID=$(docker image inspect --format '{{.Id}}' "$APP_IMAGE" 2>/dev/null) || die 'Load/build the application image before restoring.'
python3 "$REPO/deploy/backup-format.py" verify "$SOURCE" "$PROJECT" "$IMAGE_ID" || exit 1
acquire_lock
# Query inventories successfully before trusting absence; an inspect failure
# alone could mean a disconnected daemon rather than a missing resource.
CONTAINERS=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null) || die 'Cannot inventory target containers.'
VOLUMES=$(docker volume ls -q 2>/dev/null) || die 'Cannot inventory target volumes.'
PROJECT_VOLUMES=$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null) || die 'Cannot inventory project volume labels.'
NETWORKS=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null) || die 'Cannot inventory target networks.'
[[ -z "$CONTAINERS" && -z "$NETWORKS" && -z "$PROJECT_VOLUMES" ]] || die 'Target project already exists; choose a completely new project.'
while IFS= read -r volume; do
  [[ "$volume" != "${PROJECT}_postgres_data" && "$volume" != "${PROJECT}_uploads" ]] \
    || die 'Target data volume already exists; choose a completely new project.'
done <<< "$VOLUMES"
printf '%s\n' 'New environment confirmed; creating its database. Source environment is untouched.'
dc up -d --wait --wait-timeout 120 db >/dev/null 2>&1 || die 'New database startup failed; target retained for inspection.'
DB_ID=$(dc ps -q db 2>/dev/null) || die 'Cannot inspect target database.'
assert_volume_mount "$DB_ID" "${PROJECT}_postgres_data" /var/lib/postgresql/data
# SQL content is never printed, and PostgreSQL statement logging is disabled
# for this restore connection. No --clean, DROP DATABASE, or volume deletion.
dc exec -T db sh -eu -c 'export PGOPTIONS="-c log_statement=none -c log_min_error_statement=panic -c log_min_messages=panic"; exec pg_restore --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" --no-owner --no-acl --exit-on-error --single-transaction' \
  < "$SOURCE/database.dump" >/dev/null 2>&1 || die 'Database restore failed; target retained and app left stopped.'
# Create but DO NOT start app: Compose labels its new uploads volume and Docker
# initializes that volume from the image directory (UID 10001).
dc create --no-build --no-recreate --pull never app >/dev/null 2>&1 || die 'Cannot initialize target uploads; app left stopped.'
# Extraction is nonroot, cannot escape the volume and never preserves owners.
volume_tool '' 'test -z "$(ls -A /data/uploads)" && exec tar -xzf - --no-same-owner -C /data/uploads' \
  < "$SOURCE/uploads.tar.gz" >/dev/null 2>&1 || die 'Uploads restore failed; target retained and app left stopped.'
dc run --rm --no-deps --pull never -T migrate /app/admin migrate >/dev/null 2>&1 || die 'Migration failed; restored app left stopped.'
dc up -d --no-build --pull never --wait --wait-timeout 120 app >/dev/null 2>&1 || die 'Restored app did not become ready; target retained for inspection.'
printf '%s\n' 'Restore complete; new app is ready. Verify accounts, posts, photos and closed access in the browser.'
