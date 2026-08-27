#!/bin/sh
set -eu

demo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
workspace_dir=$(CDPATH= cd -- "$demo_dir/../.." && pwd)
env_file="$demo_dir/.env"

if [ ! -f "$env_file" ]; then
  echo "Missing $env_file; copy .env.example to .env and replace every placeholder." >&2
  exit 2
fi

set -a
. "$env_file"
set +a

: "${POSTGRES_PORT:=55432}"
: "${POSTGRES_USER:=aerosight}"
: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD in infra/demo/.env}"
: "${POSTGRES_DB:=aerosight}"
: "${AUTH_SECRET:?set AUTH_SECRET in infra/demo/.env}"
: "${WEB_PORT:=3100}"
: "${MQTT_PORT:=1883}"
: "${MEDIA_RTMP_PORT:=1935}"
: "${MEDIA_HLS_PORT:=8888}"
: "${MEDIA_WEBRTC_PORT:=8889}"
: "${MQTT_DEVICE_USER:?set MQTT_DEVICE_USER in infra/demo/.env}"
: "${MQTT_DEVICE_PASSWORD:?set MQTT_DEVICE_PASSWORD in infra/demo/.env}"
: "${MQTT_PLATFORM_USER:?set MQTT_PLATFORM_USER in infra/demo/.env}"
: "${MQTT_PLATFORM_PASSWORD:?set MQTT_PLATFORM_PASSWORD in infra/demo/.env}"
: "${MEDIA_PUBLISH_USER:?set MEDIA_PUBLISH_USER in infra/demo/.env}"
: "${MEDIA_PUBLISH_PASSWORD:?set MEDIA_PUBLISH_PASSWORD in infra/demo/.env}"
: "${MEDIA_ADMIN_USER:?set MEDIA_ADMIN_USER in infra/demo/.env}"
: "${MEDIA_ADMIN_PASSWORD:?set MEDIA_ADMIN_PASSWORD in infra/demo/.env}"

case "$POSTGRES_PASSWORD$AUTH_SECRET" in
  *replace-with*)
    echo "Replace the demo database password and AUTH_SECRET placeholders before starting." >&2
    exit 2
    ;;
esac

database_url="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable"
mqtt_endpoint="mqtt://127.0.0.1:$MQTT_PORT"
web_base_url="http://localhost:$WEB_PORT"
media_ingest_url="rtmp://127.0.0.1:$MEDIA_RTMP_PORT"
media_playback_url="http://127.0.0.1:$MEDIA_HLS_PORT"
webrtc_playback_url="http://127.0.0.1:$MEDIA_WEBRTC_PORT"
MEDIA_AUTH_HTTP_ADDRESS="http://host.docker.internal:$WEB_PORT/api/media-auth"
device_credentials=$(jq -nc --arg username "$MQTT_PLATFORM_USER" --arg password "$MQTT_PLATFORM_PASSWORD" '{username:$username,password:$password}')

web_pid=""
worker_pid=""
dock2_pid=""
dock3_pid=""

cleanup() {
  trap - EXIT INT TERM
  for process_id in "$dock3_pid" "$dock2_pid" "$worker_pid" "$web_pid"; do
    if [ -n "$process_id" ]; then
      kill -INT "$process_id" 2>/dev/null || true
    fi
  done
  docker ps --filter name=aerosight-dji-media --format '{{.Names}}' | while IFS= read -r container; do
    if [ -n "$container" ]; then docker stop --time 1 "$container" >/dev/null 2>&1 || true; fi
  done
}
trap cleanup EXIT INT TERM

MEDIA_AUTH_METHOD=http MEDIA_AUTH_HTTP_ADDRESS="$MEDIA_AUTH_HTTP_ADDRESS" \
docker compose --env-file "$env_file" -f "$demo_dir/compose.yaml" up -d database mqtt media

attempt=0
until docker compose --env-file "$env_file" -f "$demo_dir/compose.yaml" exec -T database \
  pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then echo "PostGIS did not become ready." >&2; exit 1; fi
  sleep 1
done

cd "$workspace_dir"
attempt=0
until DATABASE_URL="$database_url" pnpm --dir apps/web db:migrate; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 10 ]; then echo "Database migrations did not complete." >&2; exit 1; fi
  sleep 2
done

DATABASE_URL="$database_url" AUTH_SECRET="$AUTH_SECRET" \
MEDIA_PUBLISH_USER="$MEDIA_PUBLISH_USER" MEDIA_PUBLISH_PASSWORD="$MEDIA_PUBLISH_PASSWORD" \
MEDIA_ADMIN_USER="$MEDIA_ADMIN_USER" MEDIA_ADMIN_PASSWORD="$MEDIA_ADMIN_PASSWORD" \
pnpm --dir apps/web dev --port "$WEB_PORT" &
web_pid="$!"

attempt=0
until curl -fsS "$web_base_url/login" >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then echo "AeroSight Web did not become ready." >&2; exit 1; fi
  sleep 1
done

project_id=$(psql "$database_url" -X -qAt \
  -v mqtt_endpoint="$mqtt_endpoint" \
  -v web_base_url="$web_base_url" \
  -v websocket_url="ws://localhost:$WEB_PORT" \
  -v media_ingest_url="$media_ingest_url" \
  -v media_playback_url="$media_playback_url" \
  -v webrtc_playback_url="$webrtc_playback_url" \
  -f "$demo_dir/bootstrap.sql" | tail -n 1)

cd "$workspace_dir/apps/worker"
DATABASE_URL="$database_url" AUTH_SECRET="$AUTH_SECRET" \
DJI_DEMO_MQTT_CREDENTIALS="$device_credentials" \
MEDIA_API_BASE_URL="http://127.0.0.1:${MEDIA_API_PORT:-9997}" \
MEDIA_ADMIN_USER="$MEDIA_ADMIN_USER" MEDIA_ADMIN_PASSWORD="$MEDIA_ADMIN_PASSWORD" \
OBJECT_STORAGE_LOCAL_ROOT="$workspace_dir/.aerosight-objects" \
CALLBACK_LISTEN_ADDRESS="127.0.0.1:8081" \
go run ./cmd/worker &
worker_pid="$!"

attempt=0
until [ "$(psql "$database_url" -X -qAt -c "select count(*) from device_adapters where project_id=$project_id and status='connected'")" = "2" ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then echo "DJI adapters did not connect to MQTT." >&2; exit 1; fi
  sleep 1
done

start_simulator() {
  product="$1"
  gateway_sn="$2"
  aircraft_sn="$3"
  AEROSIGHT_DJI_SIM_MQTT_PASSWORD="$MQTT_DEVICE_PASSWORD" \
  go run ./cmd/simulator -mode dji-mqtt -product "$product" \
    -gateway-sn "$gateway_sn" -aircraft-sn "$aircraft_sn" \
    -mqtt-url "$mqtt_endpoint" -mqtt-username "$MQTT_DEVICE_USER" \
    -ffmpeg-executable "$demo_dir/ffmpeg-docker.sh" -media-host-override host.docker.internal &
}

start_simulator dock2-m3td DOCK2-DEMO-001 M3TD-DEMO-001
dock2_pid="$!"
start_simulator dock3-m4td DOCK3-DEMO-001 M4TD-DEMO-001
dock3_pid="$!"

echo "AeroSight DJI local demo is running."
echo "Open: $web_base_url/projects/$project_id/realtime"
echo "Login: admin@example.com / admin"
echo "Press Ctrl-C to stop app processes; run infra/demo/stop-local.sh to stop infrastructure."

while kill -0 "$web_pid" 2>/dev/null && kill -0 "$worker_pid" 2>/dev/null \
  && kill -0 "$dock2_pid" 2>/dev/null && kill -0 "$dock3_pid" 2>/dev/null; do
  sleep 2
done

echo "A demo process exited unexpectedly." >&2
exit 1
