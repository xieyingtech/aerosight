import "server-only";

import { Pool, type QueryResultRow } from "pg";

declare global {
  var aerosightPool: Pool | undefined;
}

function createPool() {
  const connectionString = process.env.DATABASE_URL;
  if (!connectionString) {
    throw new Error("DATABASE_URL is required");
  }
  return new Pool({ connectionString, max: 10 });
}

export const db = globalThis.aerosightPool ?? createPool();

if (process.env.NODE_ENV !== "production") {
  globalThis.aerosightPool = db;
}

export async function query<T extends QueryResultRow>(text: string, values: unknown[] = []) {
  return db.query<T>(text, values);
}
