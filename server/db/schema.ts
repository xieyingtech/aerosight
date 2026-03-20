import { pgTable, serial, text, timestamp } from "drizzle-orm/pg-core";

export const users = pgTable("users", {
  id: serial().primaryKey(),
  name: text().notNull(),
  email: text().unique(),
  phone: text().unique(),
  password: text(),
  role: text({ enum: ["user", "admin"] })
    .notNull()
    .default("user"),
  createdAt: timestamp("created_at").notNull().defaultNow(),
});
