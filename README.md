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
