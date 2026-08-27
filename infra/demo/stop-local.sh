#!/bin/sh
set -eu

demo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
docker compose --env-file "$demo_dir/.env" -f "$demo_dir/compose.yaml" down
