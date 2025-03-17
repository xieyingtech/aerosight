# AeroSight

## Setup

```bash
pnpm install
```

### Environment Variables

Create a `.env` file in the root of the project and add the following:

```properties
NUXT_SESSION_PASSWORD=password-with-at-least-32-characters
NUXT_PUBLIC_TIANDITU_APIKEY=your-tianditu-api-key
DATABASE_URL=postgres://localhost/aerosight
```

### Initialize Database

```bash
pnpm prisma migrate deploy
```

### Development

```bash
pnpm dev
```

### Initialize Admin User

Visit `http://localhost:3000/init` and create an admin user.
