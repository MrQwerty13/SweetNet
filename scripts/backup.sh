#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
[[ $# == 3 ]] || { printf '%s\n' 'Usage: scripts/backup.sh ENV_FILE PROJECT NEW_BACKUP_DIRECTORY' >&2; exit 2; }
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../deploy" && pwd -P)/maintenance.sh"
initialize "$1" "$2"
DEST=$3
python3 "$REPO/deploy/backup-format.py" destination "$DEST" "$REPO" || exit 1
APP_ID=$(dc ps -q app 2>/dev/null) || die 'Cannot inspect app.'
DB_ID=$(dc ps -q db 2>/dev/null) || die 'Cannot inspect database.'
[[ -n "$APP_ID" && -n "$DB_ID" ]] || die 'Start the source app and database before backup.'
[[ $(docker inspect --format '{{.State.Running}}' "$APP_ID" 2>/dev/null) == true ]] || die 'Source app must be running.'
assert_volume_mount "$APP_ID" "${PROJECT}_uploads" /data/uploads
assert_volume_mount "$DB_ID" "${PROJECT}_postgres_data" /var/lib/postgresql/data
IMAGE_ID=$(docker inspect --format '{{.Image}}' "$APP_ID" 2>/dev/null) || die 'Cannot resolve app image.'
acquire_lock
mkdir -m 700 -- "$DEST" 2>/dev/null || die 'Cannot create new backup directory; its parent must already exist.'
: > "$DEST/INCOMPLETE"
# Register recovery BEFORE stop, including a stop that partially succeeds.
RESUME_APP=1
dc stop -t 60 app >/dev/null 2>&1 || die 'Could not stop app; backup aborted.'
[[ $(docker inspect --format '{{.State.Running}}' "$APP_ID" 2>/dev/null) == false ]] || die 'App is still running; backup aborted.'
printf '%s\n' 'App stopped; copying database and uploads.'
dc exec -T db sh -eu -c 'exec pg_dump --username="$POSTGRES_USER" --dbname="$POSTGRES_DB" --format=custom --no-owner --no-acl' \
  > "$DEST/database.dump" 2>/dev/null || die 'Database backup failed; incomplete bundle retained privately.'
volume_tool ',readonly' 'exec tar -czf - -C /data/uploads .' \
  > "$DEST/uploads.tar.gz" 2>/dev/null || die 'Uploads backup failed; incomplete bundle retained privately.'
dc exec -T db pg_restore --list < "$DEST/database.dump" >/dev/null 2>&1 || die 'Database archive validation failed.'
python3 "$REPO/deploy/backup-format.py" create "$DEST" "$PROJECT" "$IMAGE_ID" || exit 1
dc start --wait --wait-timeout 120 app >/dev/null 2>&1 || die 'Backup copied, but app restart/readiness failed; recovery will retry.'
RESUME_APP=0
rm -- "$DEST/INCOMPLETE"
printf '%s\n' 'Backup complete; app is ready.'
