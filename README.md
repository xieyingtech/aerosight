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
