#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

if [ ! -f .env ]; then
  echo "copy .env.example to .env and replace every demo password first" >&2
  exit 1
fi

set -a
. ./.env
set +a

docker compose up -d --force-recreate mqtt media

docker compose --profile tools run --rm --no-deps mqtt-tools '
  mosquitto_sub -h mqtt -V mqttv5 -u "$MQTT_PLATFORM_USER" -P "$MQTT_PLATFORM_PASSWORD" -t dji/demo/verify -C 1 > /tmp/message &
  subscriber=$!
  sleep 1
  mosquitto_pub -h mqtt -V mqttv5 -u "$MQTT_DEVICE_USER" -P "$MQTT_DEVICE_PASSWORD" -t dji/demo/verify -q 1 -m authenticated
  wait "$subscriber"
  test "$(cat /tmp/message)" = authenticated
'

docker compose --profile tools pull media-tools media-check
publisher=$(docker compose --profile tools run -d --rm --no-deps media-tools \
  -hide_banner -loglevel error -re -f lavfi -i testsrc=size=320x240:rate=10 -t 60 \
  -c:v libx264 -pix_fmt yuv420p -preset ultrafast -g 10 -keyint_min 10 -sc_threshold 0 -f flv \
  "rtmp://media:1935/demo/smoke?user=$MEDIA_PUBLISH_USER&pass=$MEDIA_PUBLISH_PASSWORD")
trap 'docker stop "$publisher" >/dev/null 2>&1 || true' EXIT INT TERM

attempt=0
until docker compose --profile tools run --rm --no-deps media-check '
  token=$(printf "%s:%s" "$MEDIA_READ_USER" "$MEDIA_READ_PASSWORD" | base64)
  wget -q -T 5 -O /tmp/index.m3u8 --header "Authorization: Basic $token" http://media:8888/demo/smoke/index.m3u8
  grep -q "#EXTM3U" /tmp/index.m3u8
'; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 10 ]; then
    echo "authenticated HLS playback probe did not become ready" >&2
    exit 1
  fi
  sleep 1
done

echo "demo infrastructure verified: authenticated MQTT 5 publish/subscribe and RTMP-to-HLS playback"
