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

### Development

```bash
pnpm dev
```

### Deployment

```bash
pnpm build
pnpm preview
```
