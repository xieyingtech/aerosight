This project is built with nuxt@4 + @nuxt/hub + @nuxt/ui + @nuxtjs/i18n + nuxt-auth-utils, please refer to Context7 for their documentation.

- Most of the files from `vue`, `shared`, `utils`, `types` and nuxt modules are auto-imported by nuxt, you can directly use them in your code. DO NOT import them by yourself.

## Frontend

- Prioritize using components from Nuxt UI, and avoid writing too much tailwindcss yourself if not necessary.
- For error messages in Toast, add `e.data?.message || e.message` in the description to display the specific error information.
- Your code should be concise enough. Do not write auxiliary functions that are only used several times; instead, perform the operations directly within the template. Excluding binding events.
- New pages should use Nuxt UI page structure (`UPage`, `UPageHeader`, `UPageBody`, `UPageAside`) and keep layout/page code minimal.
- For sidebar-style pages, define navigation items with `UNavigationMenu` in layout and reuse the same items in both header body and page aside when needed.

### Example: New Page

```vue
<script setup lang="ts">
const { data: items } = await useFetch("/api/resources");
</script>

<template>
  <UPage>
    <UPageHeader title="Resources" description="Minimal list page" />
    <UPageBody>
      <pre>{{ items }}</pre>
    </UPageBody>
  </UPage>
</template>
```

### Example: Sidebar Layout

```vue
<script setup lang="ts">
import type { NavigationMenuItem } from "@nuxt/ui";

const items = computed<NavigationMenuItem[]>(() => [
  { label: "Overview", to: "/resources/1" },
  { label: "Items", to: "/resources/1/items" },
]);
</script>

<template>
  <UPage>
    <template #left>
      <UPageAside>
        <UNavigationMenu :items="items" orientation="vertical" />
      </UPageAside>
    </template>
    <slot />
  </UPage>
</template>
```

## Backend

- Authentication should use `const { user } = await getUserSession(event)` and `const { user } = await requireUserSession(event)` from nuxt-auth-utils.
- The backend API should be as simple as possible. Do not arbitrarily customize the Type. If there is no need, do not perform too much validation. Please use Zod to validate the request body. Don't define the zod schema, directly use `z.object({ ... }).parse(body)` to validate the request body.
- Database operations use the Drizzle ORM provided by NuxtHub. The database schema is defined in `server/db/schema.ts`. Use `import { db, schema } from "@nuxthub/db"` to access it.
- The backend directly returns the result returned by the database. For example, `return db.select().from(users).where(eq(users.id, userId))`.
- When there are multiple sub-paths under a certain path, directories should be created first, and then the `index.xxx.ts` should be used.
- When validating request body with Zod, return i18n message codes in error messages (e.g., `errors.validation.required`). Update the corresponding i18n files with these message codes to support multiple languages.
- Do not write the database migration files yourself. Instead, use `pnpm nuxt db generate` to generate them.
- For new backend APIs under a resource path, create directory-based routes (`server/api/<resource>/<subpath>/index.<method>.ts`) and keep handlers simple.
- For resource-scoped queries, return `404` when the target resource is not found or inaccessible.

### Example: GET API

```ts
import { db, schema } from "@nuxthub/db";
import { desc, eq } from "drizzle-orm";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const teamId = Number(getRouterParam(event, "id"));

  const team = await db
    .select({ id: schema.teams.id })
    .from(schema.teams)
    .where(eq(schema.teams.id, teamId))
    .limit(1);

  if (!team.length) {
    throw createError({ statusCode: 404, message: "errors.generic" });
  }

  return db
    .select()
    .from(schema.projects)
    .where(eq(schema.projects.teamId, teamId))
    .orderBy(desc(schema.projects.updatedAt));
});
```

### Example: POST API + Zod

```ts
import { db, schema } from "@nuxthub/db";
import { z } from "zod";

export default defineEventHandler(async (event) => {
  const { user } = await requireUserSession(event);
  const body = await readBody(event);

  const payload = z
    .object({
      name: z.string().trim().min(1, "errors.validation.teamName.required"),
    })
    .parse(body);

  return db
    .insert(schema.teams)
    .values({
      name: payload.name,
    })
    .returning();
});
```

## Middleware

- For new middleware, keep it focused on route access control and session checks only.
- If middleware only guards a feature group, place it under `server/middleware/` with a concise name and avoid adding business logic there.

### Example: Middleware

```ts
export default defineEventHandler(async (event) => {
  const { user } = await getUserSession(event);

  if (!user) {
    throw createError({ statusCode: 401, message: "errors.auth.forbidden" });
  }
});
```

After completing the editing, please run `pnpm nuxt typecheck` to check for any errors.
