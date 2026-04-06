import { db, schema } from "@nuxthub/db";

export default defineNitroPlugin(async () => {
  if (!(await db.query.users.findFirst())) {
    await db.insert(schema.users).values({
      name: "admin",
      email: "admin@example.com",
      password: await hashPassword("admin"),
      role: "admin",
    });
    console.log("Default admin user created");
  }
});
