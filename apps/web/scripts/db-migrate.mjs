import { createHash } from "node:crypto";
import { readdir, readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import pg from "pg";
import { loadLocalEnvironment } from "./load-local-env.mjs";

const { Pool } = pg;
const MIGRATION_LOCK_KEY = "aerosight.db.migrations";
const BASELINE_MIGRATION = "0001_baseline.sql";
const POSTGIS_MIGRATION = "0002_enable_postgis.sql";
const migrationsDirectory = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "../../../db/migrations"
);

function checksum(contents) {
  return createHash("sha256").update(contents).digest("hex");
}

async function loadMigrations() {
  const names = (await readdir(migrationsDirectory))
    .filter((name) => /^\d{4}_[a-z0-9_]+\.sql$/.test(name))
    .sort();

  if (names.length === 0) {
    throw new Error(`No database migrations found in ${migrationsDirectory}`);
  }

  return Promise.all(
    names.map(async (name) => {
      const sql = await readFile(resolve(migrationsDirectory, name), "utf8");
      return { name, sql, checksum: checksum(sql) };
    })
  );
}

async function ensureMigrationLedger(client) {
  await client.query(`
    create table if not exists schema_migrations (
      name text primary key,
      checksum text not null,
      adopted boolean not null default false,
      execution_ms integer not null default 0,
      applied_at timestamptz not null default now()
    )
  `);
}

async function hasLegacySchema(client) {
  const result = await client.query(
    `select to_regclass('public.users') is not null
        and to_regclass('public.projects') is not null
        and to_regclass('public.devices') is not null as present`
  );
  return result.rows[0]?.present === true;
}

export async function assertPostgisAvailable(client) {
  const result = await client.query(
    "select default_version from pg_available_extensions where name = 'postgis'"
  );
  if (result.rowCount !== 1) {
    throw new Error(
      "PostGIS extension is required but is not available; install PostGIS for this PostgreSQL server and rerun migrations"
    );
  }
}

export async function migrateDatabase({ connectionString, logger = console } = {}) {
  const databaseUrl = connectionString ?? process.env.DATABASE_URL;
  if (!databaseUrl) {
    throw new Error("DATABASE_URL is required to run database migrations");
  }

  const migrations = await loadMigrations();
  const pool = new Pool({ connectionString: databaseUrl, max: 1 });
  const client = await pool.connect();
  const appliedThisRun = [];

  try {
    await client.query("select pg_advisory_lock(hashtext($1))", [MIGRATION_LOCK_KEY]);
    await ensureMigrationLedger(client);

    const appliedResult = await client.query(
      "select name, checksum from schema_migrations order by name"
    );
    const applied = new Map(appliedResult.rows.map((row) => [row.name, row.checksum]));

    for (const migration of migrations) {
      const recordedChecksum = applied.get(migration.name);
      if (recordedChecksum) {
        if (recordedChecksum !== migration.checksum) {
          throw new Error(
            `Migration ${migration.name} changed after it was applied; restore its original contents`
          );
        }
        continue;
      }

      const adopted =
        migration.name === BASELINE_MIGRATION &&
        applied.size === 0 &&
        (await hasLegacySchema(client));
      const startedAt = performance.now();

      if (migration.name === POSTGIS_MIGRATION) {
        await assertPostgisAvailable(client);
      }

      await client.query("begin");
      try {
        if (!adopted) {
          await client.query(migration.sql);
        }
        const executionMs = Math.max(0, Math.round(performance.now() - startedAt));
        await client.query(
          `insert into schema_migrations (name, checksum, adopted, execution_ms)
           values ($1, $2, $3, $4)`,
          [migration.name, migration.checksum, adopted, executionMs]
        );
        await client.query("commit");
        applied.set(migration.name, migration.checksum);
        appliedThisRun.push({ name: migration.name, adopted, executionMs });
        logger.info?.(`${adopted ? "Adopted" : "Applied"} migration ${migration.name}`);
      } catch (error) {
        await client.query("rollback");
        throw error;
      }
    }

    return { applied: appliedThisRun, total: migrations.length };
  } finally {
    await client.query("select pg_advisory_unlock(hashtext($1))", [MIGRATION_LOCK_KEY]).catch(() => {});
    client.release();
    await pool.end();
  }
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : undefined;
if (invokedPath === import.meta.url) {
  loadLocalEnvironment();
  migrateDatabase().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}
