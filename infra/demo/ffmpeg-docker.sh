#!/bin/sh
set -eu

container_name="aerosight-dji-media-$$"
container_pid=""

cleanup() {
  if [ -n "$container_pid" ]; then
    docker stop --time 1 "$container_name" >/dev/null 2>&1 || true
    wait "$container_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

docker run --rm --name "$container_name" --entrypoint ffmpeg --add-host host.docker.internal:host-gateway \
  bluenviron/mediamtx:1.20.1-ffmpeg "$@" &
container_pid="$!"
wait "$container_pid"
