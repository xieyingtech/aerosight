import { hash } from "bcryptjs";
import pg from "pg";

const email = process.env.ADMIN_EMAIL;
const password = process.env.ADMIN_PASSWORD;

if (!process.env.DATABASE_URL || !email || !password) {
  throw new Error("DATABASE_URL, ADMIN_EMAIL and ADMIN_PASSWORD are required");
}

const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL });
try {
  const passwordHash = await hash(password, 12);
  await pool.query(
    `insert into users (name, email, password, role)
     values ($1, $2, $3, 'admin')
     on conflict (email) do update
       set password = excluded.password, role = 'admin', updated_at = now()`,
    [process.env.ADMIN_NAME ?? "admin", email, passwordHash]
  );
  console.log(`Admin user ${email} is ready.`);
} finally {
  await pool.end();
}
