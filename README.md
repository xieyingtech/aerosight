# AeroSight

空地一体化智能感知平台。

## Structure

- `apps/web`: Next.js App Router UI, exported as static pages; browser requests use the Go API.
- `apps/server`: Gin HTTP API, authentication, database access, embedded frontend and background workers in one application.
- `db`: PostgreSQL/PostGIS migrations and the schema snapshot.

## Development

1. Install Node.js, pnpm and Go, then run `pnpm install`.
2. Copy the root `.env.example` to `.env.local`. Set `DATABASE_URL`, an `AUTH_SECRET` of at least 32 characters, and `CSRF_AUTH_KEY` (32 random bytes encoded as base64).
3. Start PostgreSQL with PostGIS enabled. `pnpm db:migrate` runs the Go migration command and is safe to repeat.
4. Run `pnpm dev`. It builds Go with the `dev` tag, starts the API on `127.0.0.1:8080` and Next dev on port 3000. Next proxies `/api/*` and `/algorithm-assets/*` to Go. Set `PUBLIC_ORIGIN` to the browser's Next origin; `GO_API_ORIGIN` is the internal API target.

For separate terminals, use `pnpm dev:server` and `pnpm dev:web`. The combined command loads the root `.env.local`; a standalone Next command needs `GO_API_ORIGIN` in its environment when the target differs from the default. Go owns migrations and administrator initialization. On an empty users table, the existing bootstrap creates `admin@example.com` with password `admin`.

## Checks

```bash
pnpm check
pnpm build
pnpm db:check
pnpm test:migrations
```

Go unit tests use the `dev` tag so a frontend export is not required. Production embed tests can be run after `pnpm build` with `go test ./internal/webassets` from `apps/server`.

`pnpm check:web-boundary` prevents server dependencies, Route Handlers, Server Actions and legacy database imports from returning to the Web application. Historical SQL contract fixtures live in `contracts/go-migration/legacy-web`; `pnpm test:web` includes their tests. Database regression, benchmark and rollback tools live in `scripts/legacy-db` and use the root development dependency `pg`. The Web package has no `pg` dependency; production migrations run in Go.

`pnpm drill:fresh-environment` checks the documented unified-service environment keys, configuration contracts, disposable database migrations and the full production build. It runs from the repository root even when invoked elsewhere and supports Windows pnpm launching. AI providers are managed in database configuration; obsolete `AI_PROVIDER`/`AI_MODEL` variables are not required. Its success covers configuration, migration and build only; the complete simulated-device/task/algorithm/AI business drill is a separate acceptance gate.

After `pnpm build`, `pnpm drill:upgrade-rollback` creates a disposable PostGIS database, uses the legacy TS migrator through migration 0051, then runs the copied production Go executable to apply pending migrations. It checks unchanged historical ledger fields, a zero-change repeat, legacy read queries, evidence retention and unknown-event preservation. Evidence is saved in `.build/upgrade-rollback-<id>/`. This is database compatibility acceptance: it does not start the old Web/worker release, and its result explicitly keeps application rollback unverified.

To exercise the full application cutover, pass `--legacy-release <directory>`. That directory must contain its original `apps/web` production build and dependencies, plus `.build/aerosight-worker` (`.exe` on Windows). The tested legacy source is commit `bc314b3731ebc27b301ce0e15d0ffd50ab090a40`, exported with `git archive` and installed with its frozen pnpm lockfile. Build its Next app using a non-secret test `AUTH_SECRET` and a build-only `DATABASE_URL`, and compile its original `apps/worker/cmd/worker` into that directory's `.build`. The drill creates an account before upgrade, then cross-compiles the current Go executable and runs it in the cached `nginx:alpine` runtime fixture with the entrypoint replaced. It verifies Go login, a newly created project, SSE and successful SIGTERM exit with no remaining database connections. Only then does it start the actual old Next production server and worker, verify the old password, original project/snapshot and Go-created project, and save screenshots and process logs. It needs a Go compiler, Docker and a Playwright browser. The new Go service's real SIGTERM is checked; Windows cleanup of the final old processes is forced and is not old-worker graceful-shutdown acceptance. The runtime fixture does not replace final release-image validation.

`pnpm test:dev-proxy` runs the actual Next/Go development launcher against an isolated PostGIS Docker container. It requires installed dependencies, Go and Docker, and checks authentication, CSRF, cookies, signed assets, Range/HEAD and SSE cancellation through Next rewrites. It stops its processes and test database afterward; logs are retained under `.build/dev-proxy-<id>/`. On Windows the test uses process-tree termination for cleanup; this does not test graceful production shutdown.

`pnpm test:mqtt-lifecycle` uses an isolated authenticated Mosquitto broker to check MQTT 5 authentication, reconnection/subscription recovery, and adapter-manager shutdown/restart. First pull the pinned image with `docker pull eclipse-mosquitto:2.1.2-alpine`; Go and Docker are required. It binds a random loopback port, generates test credentials, and removes its container afterward. Broker/test logs and results remain in `.build/mqtt-lifecycle-<id>/`. The manager test uses a lease repository fixture with real MQTT connections; it does not replace database lease or whole-process shutdown acceptance.

`pnpm test:media-inspector` runs the Go MediaMTX Inspector integration test against `bluenviron/mediamtx:1.20.1`, with authenticated management API access and a real FFmpeg H264 publisher. It verifies anonymous API rejection and actual track detection, saves evidence in `.build/media-inspector-<id>/`, and cleans up its publisher/container. It requires Docker, Go and FFmpeg with libx264; only the designated synthetic RTSP publishing path permits anonymous publishing in this fixture. Browser playback is covered separately by `test:production-browser`.

The container lifecycle and release-image tests also hold 24 concurrent project SSE connections while issuing 80 snapshot requests in batches of eight. They sample database connections against the default combined pool budget of 30, publish a new event after 32 seconds and require every stream to receive it, then verify all streams close on SIGTERM and database connections reach zero before restart. This measures HTTP/SSE shutdown under concurrent reads; recovery of active task/algorithm/AI side effects remains part of the broader workload drill.

After `pnpm build:web`, `pnpm test:container-lifecycle` cross-compiles the production Go binary and runs it as an unprivileged PID 1 with a read-only filesystem, mounting only that binary. The test uses the locally cached `nginx:alpine` image with its entrypoint replaced (nginx is not started), checks that Node/pnpm are absent and Go owns one TCP listener, and creates an isolated PostGIS container/network. It verifies embedded pages, login and writes, SSE closure on SIGTERM, successful exit within 30 seconds, database connection release, and session/project persistence after restart. Logs and results remain in `.build/container-lifecycle-<id>/`. The HTTP client supplies the configured public Origin and cookies directly to the backend; browser HTTPS/cookie enforcement is covered separately. This runtime fixture does not replace acceptance of the release Dockerfile or MQTT/media workloads.

Build the release image with `docker build -t aerosight:unified-acceptance .`, then run `pnpm test:release-image`. This uses the same lifecycle assertions against the image's own binary and default user/entrypoint, with no host binary or source mounts. It checks image user, command and exposed-port metadata before startup. To test another built image, run `node scripts/test-container-lifecycle.mjs --image <image>`. Results explicitly distinguish `release-image` from the `mounted-binary` fixture mode.

After `pnpm build`, `pnpm test:production-browser` uses the production Go binary, disposable PostGIS and local HTTPS termination fixtures to test login hydration, Secure session cookies, team/project creation through the UI, post-build resource detail reloads, workspace navigation, authorization failures, logout and session expiry. It also verifies MapLibre worker rendering, signed playback authorization, real MediaMTX WebRTC video decoding, HLS fallback decoding, blocked media frames and CSP rejection of an unapproved inline script. It requires Docker with `bluenviron/mediamtx:1.20.1`, OpenSSL, FFmpeg with libx264 and a Playwright browser with native HLS support; verified on installed Windows Edge. `PLAYWRIGHT_CHANNEL` can select another installed channel; the default on other platforms is Playwright Chromium, whose native HLS support must be checked before running the media acceptance. Screenshots, results and logs remain in `.build/production-browser-<id>/`. The temporary certificate is trusted only by the test browser context. WebRTC uses a synthetic RTSP publisher and real ICE/TCP transport; its read authorization reaches Go, while only the designated synthetic publish path bypasses authentication in the test broker. HLS uses a generated six-second VOD fixture, and the map uses a controlled style. This does not verify public tile availability, public-network NAT traversal, device publish credentials or graceful shutdown. Test media containers, publishers and TLS/auth bridges are cleaned up and are not production application services.

## Production startup

`pnpm test:development-browser` runs the same resource and session lifecycle interactions through the actual Next development launcher, using HTTP and development cookies. It needs Docker and a Playwright browser, but no OpenSSL. Next's `allowedDevOrigins` follows the hostname in development `PUBLIC_ORIGIN`, including `127.0.0.1` when configured. Failure screenshots and details are saved alongside the service log.

The production browser run also checks loading, retry and API-denial UI states for projects, teams, profile and all administration pages using controlled response fixtures. It separately revokes the actual database administrator role and verifies both the layout guard and the API reject access.

Both browser modes verify all 16 legacy page mappings with GET/HEAD, conflicting IDs, repeated filters and invalid IDs, then exercise browser back/forward navigation through a redirected project detail. Results are recorded in `legacy-links.json`.

The production run also verifies a real database device pose rendered and selected on the map under CSP, including a MapLibre blob worker. Its external map style response is a deterministic fixture; it does not depend on public demo tiles. Map evidence is stored in `map.json` and `map-*.png`; WebRTC and HLS playback evidence is stored in `media-frame.json` and the corresponding playback screenshots.

```bash
pnpm build
pnpm start
```

`pnpm build` runs TypeScript checks, exports Next, checks sqlc generation for drift, validates and copies frontend/migration assets, then compiles `.build/aerosight` (`aerosight.exe` on Windows). A failed check stops the build before the Go compilation. `pnpm start` launches only this Go application. Go applies pending migrations before serving HTTP and running background workers.

The deployment executable embeds frontend pages and migrations; it can run without Node.js or the source checkout:

```bash
./aerosight migrate
./aerosight serve
```

Supply environment variables directly when running the executable. Use `AEROSIGHT_ENV=production`, an HTTPS `PUBLIC_ORIGIN`, and an appropriate `HTTP_LISTEN_ADDRESS` behind the deployment TLS endpoint. Database, MQTT and MediaMTX remain external services. The separate callback listener belongs to the legacy worker maintenance command; unified `serve` handles callbacks on its HTTP port.

Static HTML receives a page-specific CSP with build-verified inline script hashes. `CSP_MAP_ORIGINS` defaults to `https://demotiles.maplibre.org`; `CSP_MEDIA_ORIGINS` lists the browser-visible MediaMTX origins used for playback. Both accept comma-separated HTTPS origins without paths, credentials or wildcards (HTTP is allowed in development). Media origins allow connections, images, video and playback frames, but never external scripts. Map workers may use `blob:`; inline styles remain enabled for the UI. Go development mode serves no frontend pages, so Next continues to manage its own HMR resources.

The root Dockerfile builds the Next export and Go executable in separate stages. Its final image contains the Go executable, CA certificates and IANA timezone data, runs as UID/GID 10001, and exposes application port 8080. The CA bundle and timezone database are copied from the official Go Bookworm build image into the Debian Bookworm runtime, so the runtime stage does not need another package download. Release acceptance checks the CA bundle and the `Asia/Shanghai` UTC offset. Node.js and the source checkout are not copied into the runtime image. Build context rules exclude local environment files and generated artifacts.

```bash
docker build -t aerosight:local .
docker run --rm --env-file /path/to/production.env aerosight:local migrate
docker run --name aerosight --env-file /path/to/production.env \
  -p 127.0.0.1:8080:8080 -v aerosight-objects:/var/lib/aerosight/objects \
  --stop-timeout 35 aerosight:local
```

The production environment file must provide `DATABASE_URL`, the existing `AUTH_SECRET`, `CSRF_AUTH_KEY`, and HTTPS `PUBLIC_ORIGIN`. Preserve the image's `HTTP_LISTEN_ADDRESS=0.0.0.0:8080` and `AEROSIGHT_ENV=production`; do not reuse the development example unchanged. Point database/MQTT/media addresses to hosts reachable from the container. Configure `CALLBACK_PUBLIC_BASE_URL` for the same public application endpoint when needed. A bind-mounted object directory must be writable by UID 10001; preserve object data across releases. Terminate TLS at the deployment endpoint and allow at least the configured `SHUTDOWN_TIMEOUT` (default 30 seconds) before forcibly stopping the container.

For an upgrade, retain the exact old Web/worker release and its configuration, including the original `AUTH_SECRET`. Take a verified database backup and an object-storage backup or versioned snapshot; both must describe the same cutover state. Stop the old Web and worker before running the new executable's `migrate`, then start `serve`. Do not allow both application versions to write concurrently. Check `/readyz`, login, tenant permissions, SSE, task processing and external callbacks before switching traffic. Users must sign in again because old Auth.js cookies are not Go sessions; account passwords and encrypted integration credentials retain their existing values and key semantics.

For application rollback, stop the new service and verify its workers and connections have drained, then restore the retained old Web/worker release and original configuration. Preserve the upgraded database and object data, including the additive `sessions` table; do not run destructive down migrations or blindly restore an older backup over post-upgrade writes. Verify the old release against the upgraded schema and its supported event types before returning traffic. Database disaster recovery is a separate coordinated restore of database and objects, with an explicit recovery point. The optional application drill above exercises this service switch in an isolated database; it does not replace deployment-specific backup restoration or the remaining release-image and full-load gates.

## Spec-driven development

This repository uses [OpenSpec](https://openspec.dev/) for non-trivial feature,
behavior, data-model, API, and authorization changes. OpenSpec artifacts are
written in Chinese and committed alongside the code.

In Codex, use the generated skills in this order:

1. `$openspec-explore` to investigate or clarify an idea without changing code.
2. `$openspec-propose <change description>` to create the proposal, delta specs,
   design, and implementation tasks.
3. Review the generated artifacts, then start a new request with
   `$openspec-apply-change <change name>` to implement it.
4. Use `$openspec-sync-specs <change name>` and
   `$openspec-archive-change <change name>` after implementation and verification.

The CLI is available through pnpm:

```bash
pnpm spec list
pnpm spec status --change <change-name>
pnpm spec validate <change-name>
```

Active changes live in `openspec/changes/`; current system specifications live
in `openspec/specs/` after changes are synchronized and archived.
