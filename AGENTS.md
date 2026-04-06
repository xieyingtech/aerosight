This project is built with nuxt@4 + @nuxt/hub + @nuxt/ui + @nuxtjs/i18n + nuxt-auth-utils, please refer to Context7 for their documentation.

- Most of the files from `vue`, `shared`, `utils`, `types` and nuxt modules are auto-imported by nuxt, you can directly use them in your code. DO NOT import them by yourself.

## Frontend

- Prioritize using components from Nuxt UI, and avoid writing too much tailwindcss yourself if not necessary.
- For error messages in Toast, add `e.data?.message || e.message` in the description to display the specific error information.
- Your code should be concise enough. Do not write auxiliary functions that are only used several times; instead, perform the operations directly within the template.

## Backend

- Authentication should use `await getUserSession(event)` and `await requireUserSession(event)` from nuxt-auth-utils.
- The backend API should be as simple as possible. Do not arbitrarily customize the Type. If there is no need, do not perform too much validation. Please use Zod to validate the request body. Don't define the zod schema, directly use `z.object({ ... }).parse(body)` to validate the request body.
- Database operations use the Drizzle ORM provided by NuxtHub. The database schema is defined in `server/db/schema.ts`. Use `import { db, schema } from "@nuxthub/db"` to access it.
- The backend directly returns the result returned by the database. For example, `return db.select().from(users).where(eq(users.id, userId))`.
- When there are multiple sub-paths under a certain path, directories should be created first, and then the `index.xxx.ts` should be used.
- When validating request body with Zod, return i18n message codes in error messages (e.g., `errors.validation.required`). Update the corresponding i18n files with these message codes to support multiple languages.
