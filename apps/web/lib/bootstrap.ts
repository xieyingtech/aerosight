import "server-only";

import { hash } from "bcryptjs";
import { db } from "@/lib/db";

const DEFAULT_ADMIN_EMAIL = "admin@example.com";
const DEFAULT_ADMIN_PASSWORD = "admin";

export async function ensureDefaultAdmin() {
  const client = await db.connect();

  try {
    await client.query("begin");
    await client.query("select pg_advisory_xact_lock(hashtext($1))", [
      "aerosight.bootstrap.default-admin"
    ]);

    const users = await client.query("select 1 from users limit 1");
    if (users.rowCount === 0) {
      const passwordHash = await hash(DEFAULT_ADMIN_PASSWORD, 12);
      await client.query(
        `insert into users (name, email, password, role)
         values ('admin', $1, $2, 'admin')`,
        [DEFAULT_ADMIN_EMAIL, passwordHash]
      );
      console.info(`Created default administrator ${DEFAULT_ADMIN_EMAIL}.`);
    }

    await client.query("commit");
  } catch (error) {
    await client.query("rollback");
    throw error;
  } finally {
    client.release();
  }
}
