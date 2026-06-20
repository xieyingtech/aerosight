# AeroSight

Intelligent drone patrol management system.

## Stack

- API: Go + Gin + pgx, with sqlc query definitions in `db/queries`.
- Web: Next.js App Router + React + Tailwind CSS + MapLibre.
- Database: PostgreSQL. The development schema is in `db/schema.sql`.

## Development

1. Create a PostgreSQL database and apply `db/schema.sql`.
2. Copy `.env.example` to `.env` and fill `DATABASE_URL` and `SESSION_SECRET`.
3. Start the Go API:

```bash
go run ./cmd/api
```

4. Start the Next.js app:

```bash
cd web
pnpm install
pnpm dev
```

The default admin user is created on API startup when the `users` table is empty:

- Email: `admin@example.com`
- Password: `admin`

## Checks

```bash
go test ./...
cd web
pnpm typecheck
pnpm build
```
