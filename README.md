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

`pnpm test:dev-proxy` runs the actual Next/Go development launcher against an isolated PostGIS Docker container. It requires installed dependencies, Go and Docker, and checks authentication, CSRF, cookies, signed assets, Range/HEAD and SSE cancellation through Next rewrites. It stops its processes and test database afterward; logs are retained under `.build/dev-proxy-<id>/`. On Windows the test uses process-tree termination for cleanup; this does not test graceful production shutdown.

## Production startup

```bash
pnpm build
pnpm start
```

`pnpm build` exports Next, validates and copies frontend/migration assets, then compiles `.build/aerosight` (`aerosight.exe` on Windows). `pnpm start` launches only this Go application. Go applies pending migrations before serving HTTP and running background workers.

The deployment executable embeds frontend pages and migrations; it can run without Node.js or the source checkout:

```bash
./aerosight migrate
./aerosight serve
```

Supply environment variables directly when running the executable. Use `AEROSIGHT_ENV=production`, an HTTPS `PUBLIC_ORIGIN`, and an appropriate `HTTP_LISTEN_ADDRESS` behind the deployment TLS endpoint. Database, MQTT and MediaMTX remain external services. The separate callback listener belongs to the legacy worker maintenance command; unified `serve` handles callbacks on its HTTP port.

The root Dockerfile builds the Next export and Go executable in separate stages. Its final image contains the Go executable and CA certificates, runs as UID/GID 10001, and exposes application port 8080. Node.js and the source checkout are not copied into the runtime image. Build context rules exclude local environment files and generated artifacts.

```bash
docker build -t aerosight:local .
docker run --rm --env-file /path/to/production.env aerosight:local migrate
docker run --name aerosight --env-file /path/to/production.env \
  -p 127.0.0.1:8080:8080 -v aerosight-objects:/var/lib/aerosight/objects \
  --stop-timeout 35 aerosight:local
```

The production environment file must provide `DATABASE_URL`, the existing `AUTH_SECRET`, `CSRF_AUTH_KEY`, and HTTPS `PUBLIC_ORIGIN`. Preserve the image's `HTTP_LISTEN_ADDRESS=0.0.0.0:8080` and `AEROSIGHT_ENV=production`; do not reuse the development example unchanged. Point database/MQTT/media addresses to hosts reachable from the container. Configure `CALLBACK_PUBLIC_BASE_URL` for the same public application endpoint when needed. A bind-mounted object directory must be writable by UID 10001; preserve object data across releases. Terminate TLS at the deployment endpoint and allow at least the configured `SHUTDOWN_TIMEOUT` (default 30 seconds) before forcibly stopping the container.

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
