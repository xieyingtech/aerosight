# AeroSight

空地一体化智能感知平台。

## Structure

- `apps/web`: Next.js App Router, Auth.js, PostgreSQL data access and the application UI.
- `apps/worker`: Minimal Go worker process. Queue consumers will be added with the first background task.
- `db`: PostgreSQL schema.
- Database: PostgreSQL. The development schema is in `db/schema.sql`.

## Development

1. Create a PostgreSQL database and apply all pending migrations:

```bash
pnpm db:migrate
```

`db/schema.sql` remains the complete schema snapshot. Existing installations
that already match this snapshot safely adopt the baseline migration on their
first migration run.
2. Copy `apps/web/.env.example` to `.env.local` at the repository root and fill
   `DATABASE_URL` and `AUTH_SECRET`. Then either copy it to
   `apps/web/.env.local` or symlink that path to the root `.env.local`.
3. Install the web dependencies:

```bash
pnpm install
```

4. Start Next.js and the worker:

```bash
pnpm dev
```

They can also be started independently with `pnpm dev:web` and
`pnpm dev:worker`.

When Next.js starts with an empty `users` table, it creates the default
administrator `admin@example.com` with password `admin`.

Next.js owns authentication, authorization and synchronous database operations.
The Go process is reserved for asynchronous work delivered through PostgreSQL
and, when needed, Redis.

## Checks

```bash
pnpm check
pnpm build
pnpm test:migrations
```
