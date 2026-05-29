#!/bin/sh
set -eu

demo_env="${SIL_APP_ENV:-development}"

mkdir -p /app/var
rm -rf "/app/var/${demo_env}"

printf '%s\n' 'running demo ingest'
/app/bin/ingestd

printf '%s\n' 'running demo normalize'
/app/bin/normalized

printf '%s\n' 'running demo index'
/app/bin/indexerd

printf '%s\n' 'starting demo api'
exec /app/bin/apid
