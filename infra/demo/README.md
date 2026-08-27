# AeroSight demo infrastructure

This isolated Compose stack provides the protocol dependencies for the DJI demo path. It is intentionally not exposed through the root `package.json`.

1. Copy `.env.example` to `.env` and replace every password with a random value. Credentials are used only to generate files inside a Docker volume and are not committed.
2. Set `MEDIA_WEBRTC_ADDITIONAL_HOSTS` to the LAN address that browsers use to reach this machine.
3. Start and verify:

```sh
cd infra/demo
docker compose up -d mqtt media
./verify.sh
```

The verification publishes an authenticated MQTT 5 QoS 1 message, then publishes an H.264 test pattern over authenticated RTMP and reads its authenticated HLS manifest. Stop the stack with `docker compose down`; add `--volumes` only when intentionally deleting generated credentials and broker persistence.

LAN demo ports default to MQTT `1883`, RTMP `1935`, HLS `8888`, WebRTC HTTP `8889`, WebRTC ICE/UDP `8189`, and the MediaMTX control API `9997`. These listeners are for a trusted demo network. Public deployment requires TLS termination, per-project credentials, topic/path ACLs, and restricted API exposure.
