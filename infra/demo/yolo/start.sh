#!/bin/sh
set -eu
cd "$(dirname "$0")/../../.."
mkdir -p .build/msup-demo/ultralytics
export YOLO_CONFIG_DIR="$PWD/.build/msup-demo/ultralytics"
exec .build/msup-demo/yolo-venv/bin/python -m uvicorn server:app --app-dir infra/demo/yolo --host 127.0.0.1 --port "${YOLO_PORT:-8091}"
