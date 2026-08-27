# AeroSight DJI local demo

This isolated entry starts AeroSight, PostGIS, an authenticated MQTT broker, MediaMTX, and protocol-level simulators for Dock 2 + Matrice 3TD and Dock 3 + Matrice 4TD. The simulators publish DJI Cloud API MQTT envelopes and H.264 RTMP streams; they do not write the database or call an internal success shortcut. The entry is intentionally not exposed through the root `package.json`.

LAN and public endpoint examples, firewall rules, TLS requirements, Topic ACLs and media permissions are documented in [the device network profile guide](../../docs/operations/device-network-profiles.md).

## Run the complete demo

Requirements: Docker with Compose, Node.js/pnpm, Go, `psql`, `curl`, and `jq`.

1. Copy `.env.example` to `.env` and replace every placeholder password and `AUTH_SECRET`. Credentials are used only to generate files inside a Docker volume and are not committed.
2. Keep `MEDIA_WEBRTC_ADDITIONAL_HOSTS=127.0.0.1` for a browser on the same machine, or set it to the LAN address used by remote browsers.
3. From the repository root, run:

```sh
./infra/demo/run-local.sh
```

Open the printed URL and sign in with the printed local seed account. The script creates one project, a LAN network profile, and two DJI adapters. Device discovery then creates every dock, aircraft, camera, and sensor through the shared Device → DeviceType → Driver path.

Use the device tree to verify `dji.cloud` Driver binding and effective capabilities. Select a camera and use **启动直播**; the selected simulated camera emits a distinct test pattern through authenticated RTMP and is played through token-protected WebRTC. Use **停止直播** to verify DJI stop acknowledgement and media cleanup. Select a controllable aircraft or dock to exercise return-home and capability-driven dock actions. Sensor and telemetry channels appear in the same real-time panel without device-category UI branches.

Press Ctrl-C to stop Web, Worker, and simulator processes. Then stop infrastructure with:

```sh
./infra/demo/stop-local.sh
```

The database volume is retained between runs. Add `--volumes` to the Compose down command only when intentionally deleting local demo data and generated credentials.

## Infrastructure-only smoke test

To verify MQTT and media dependencies without the application:

```sh
cd infra/demo
docker compose up -d mqtt media
./verify.sh
```

This publishes an authenticated MQTT 5 QoS 1 message, then publishes an H.264 test pattern over authenticated RTMP and reads its authenticated HLS manifest.

`run-local.sh` configures MediaMTX HTTP authentication automatically. MediaMTX calls `/api/media-auth`; browser HLS/WebRTC URLs are accepted only while their path-scoped token is valid. Keep internal authentication only for the standalone infrastructure smoke test above.

LAN demo ports default to MQTT `1883`, RTMP `1935`, HLS `8888`, WebRTC HTTP `8889`, WebRTC ICE/UDP `8189`, and the MediaMTX control API `9997`. These listeners are for a trusted demo network. Public deployment requires TLS termination, per-project credentials, topic/path ACLs, and restricted API exposure.
