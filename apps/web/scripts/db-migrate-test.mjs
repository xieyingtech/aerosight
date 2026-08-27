import { randomBytes } from "node:crypto";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

import pg from "pg";

import { assertPostgisAvailable, migrateDatabase } from "./db-migrate.mjs";

const { Client } = pg;
const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const legacySchema = await readFile(
  resolve(repositoryRoot, "db/migrations/0001_baseline.sql"),
  "utf8"
);
const currentSchema = await readFile(resolve(repositoryRoot, "db/schema.sql"), "utf8");
const integrationFixture = JSON.parse(
  await readFile(resolve(repositoryRoot, "test/fixtures/air-ground-projects.json"), "utf8")
);
const silentLogger = { info() {} };
let adminUrl;

function docker(...args) {
  const result = spawnSync("docker", args, { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(result.stderr.trim() || `docker ${args[0]} failed`);
  }
  return result.stdout.trim();
}

async function startTestPostgis() {
  if (process.env.MIGRATION_TEST_DATABASE_URL) {
    return { url: process.env.MIGRATION_TEST_DATABASE_URL, cleanup() {} };
  }

  const name = `aerosight-postgis-test-${randomBytes(5).toString("hex")}`;
  docker(
    "run",
    "--detach",
    "--rm",
    "--platform",
    "linux/amd64",
    "--name",
    name,
    "--env",
    "POSTGRES_PASSWORD=aerosight-test",
    "--publish",
    "127.0.0.1::5432",
    "postgis/postgis:17-3.5"
  );
  const portOutput = docker("port", name, "5432/tcp");
  const port = portOutput.match(/:(\d+)$/)?.[1];
  if (!port) throw new Error(`Could not determine PostgreSQL port from: ${portOutput}`);
  const url = `postgresql://postgres:aerosight-test@127.0.0.1:${port}/postgres`;

  let lastError;
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const client = new Client({ connectionString: url });
    try {
      await client.connect();
      await client.end();
      return { url, cleanup: () => docker("stop", name) };
    } catch (error) {
      lastError = error;
      await client.end().catch(() => {});
      await new Promise((resolveDelay) => setTimeout(resolveDelay, 500));
    }
  }

  docker("stop", name);
  throw lastError ?? new Error("Timed out waiting for PostGIS test database");
}

function databaseUrl(name) {
  const url = new URL(adminUrl);
  url.pathname = `/${name}`;
  return url.toString();
}

async function withTemporaryDatabase(label, test) {
  const name = `aerosight_migration_${label}_${randomBytes(5).toString("hex")}`;
  const admin = new Client({ connectionString: adminUrl });
  await admin.connect();
  await admin.query(`create database "${name}"`);

  try {
    await test(databaseUrl(name));
  } finally {
    await admin.query(`drop database if exists "${name}" with (force)`);
    await admin.end();
  }
}

async function readMigrationState(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const tables = await client.query(
      `select to_regclass('public.users') as users,
              to_regclass('public.projects') as projects,
              to_regclass('public.devices') as devices,
              postgis_full_version() as postgis_version,
              st_astext('SRID=4326;POINT Z (120.1 30.2 88)'::geometry(PointZ, 4326)) as point_z`
    );
    const migrations = await client.query(
      "select name, checksum, adopted from schema_migrations order by name"
    );
    return { tables: tables.rows[0], migrations: migrations.rows };
  } finally {
    await client.end();
  }
}

async function assertLegacyProjectDefaults(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    await client.query("insert into users (name, email) values ('legacy-user', 'legacy@example.test')");
    await client.query("insert into teams (name) values ('legacy-team')");
    await client.query(
      `insert into team_members (team_id, user_id, role)
       select teams.id, users.id, 'owner' from teams cross join users
       where teams.name = 'legacy-team' and users.email = 'legacy@example.test'`
    );
    await client.query(
      `insert into projects (team_id, name)
       select id, 'legacy-project' from teams where name = 'legacy-team'`
    );
    const result = await client.query(
      `select project.name,
              coalesce(flags.device_commands_enabled, false) as device_commands,
              coalesce(flags.operations_overview_enabled, false) as operations_overview,
              coalesce(flags.object_storage_enabled, false) as object_storage,
              coalesce(flags.external_algorithms_enabled, false) as external_algorithms,
              coalesce(flags.automatic_ai_enabled, false) as automatic_ai
         from projects project
         left join project_feature_flags flags on flags.project_id = project.id
        where project.name = 'legacy-project'`
    );
    const row = result.rows[0];
    assert(row?.name === "legacy-project", "legacy project query stopped working");
    assert(
      [row.device_commands, row.operations_overview, row.object_storage, row.external_algorithms, row.automatic_ai].every(
        (value) => value === false
      ),
      "new project capabilities must default to disabled"
    );
  } finally {
    await client.end();
  }
}

async function assertProjectIsolationFixture(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    for (const project of integrationFixture.projects) {
      const user = await client.query(
        "insert into users (name, email) values ($1, $2) returning id",
        [project.owner.name, project.owner.email]
      );
      const team = await client.query("insert into teams (name) values ($1) returning id", [project.teamName]);
      await client.query(
        "insert into team_members (team_id, user_id, role) values ($1, $2, 'owner')",
        [team.rows[0].id, user.rows[0].id]
      );
      const insertedProject = await client.query(
        "insert into projects (team_id, name, created_by_user_id) values ($1, $2, $3) returning id",
        [team.rows[0].id, project.name, user.rows[0].id]
      );
      for (const device of project.devices) {
        await client.query(
          `insert into devices (project_id, name, type, status, metadata_json)
           values ($1, $2, $3, 'offline', jsonb_build_object('externalId', $4::text))`,
          [insertedProject.rows[0].id, device.name, device.type, device.externalId]
        );
      }
    }

    for (const project of integrationFixture.projects) {
      const visible = await client.query(
        `select project.name as project_name, device.name as device_name
           from projects project
           join team_members membership on membership.team_id = project.team_id
           join users actor on actor.id = membership.user_id
           join devices device on device.project_id = project.id
          where actor.email = $1`,
        [project.owner.email]
      );
      assert(visible.rowCount === project.devices.length, "project member saw an unexpected device count");
      assert(
        visible.rows.every((row) => row.project_name === project.name),
        "project member query crossed the project boundary"
      );
    }

    const crossProject = await client.query(
      `select project.id as project_id, project.team_id, actor.id as foreign_user_id
         from projects project
         join users actor on actor.email = 'south-operator@example.test'
        where project.name = '北区巡检'`
    );
    const target = crossProject.rows[0];
    await client.query(
      `insert into project_permissions (project_id, team_id, user_id, permission)
       values ($1, $2, $3, 'event:handle')`,
      [target.project_id, target.team_id, target.foreign_user_id]
    ).then(
      () => assert(false, "cross-team project permission should violate membership constraint"),
      (error) => assert(error.code === "23503", "cross-team permission failed for an unexpected reason")
    );

    const guessedResource = await client.query(
      `select device.id
         from devices device
         join projects project on project.id = device.project_id
         join team_members membership on membership.team_id = project.team_id
         join users actor on actor.id = membership.user_id
        where actor.email = 'north-operator@example.test'
          and project.name = '北区巡检'
          and device.id = (
            select foreign_device.id
              from devices foreign_device
              join projects foreign_project on foreign_project.id = foreign_device.project_id
             where foreign_project.name = '南区巡检'
             limit 1
          )`
    );
    assert(guessedResource.rowCount === 0, "scoped resource lookup exposed another project resource ID");
  } finally {
    await client.end();
  }
}

async function assertConcurrentIdempotency(connectionString) {
  const setup = new Client({ connectionString });
  await setup.connect();
  await setup.query("create table idempotency_test_effects (id bigserial primary key, marker text not null)");
  const scope = await setup.query(
    "select id as project_id, team_id from projects where name = 'legacy-project'"
  );
  await setup.end();
  const { project_id: projectId, team_id: teamId } = scope.rows[0];

  const first = new Client({ connectionString });
  const second = new Client({ connectionString });
  await first.connect();
  await second.connect();
  try {
    await first.query("begin");
    const owner = await first.query(
      `insert into idempotency_records (
         project_id, team_id, actor_key, operation, idempotency_key, request_hash
       ) values ($1, $2, 'user:fixture', 'test.effect', 'same-key', 'same-request')
       on conflict (project_id, actor_key, operation, idempotency_key) do nothing
       returning id`,
      [projectId, teamId]
    );
    assert(owner.rowCount === 1, "first concurrent request did not own the idempotency record");

    const contender = second.query(
      `insert into idempotency_records (
         project_id, team_id, actor_key, operation, idempotency_key, request_hash
       ) values ($1, $2, 'user:fixture', 'test.effect', 'same-key', 'same-request')
       on conflict (project_id, actor_key, operation, idempotency_key) do nothing
       returning id`,
      [projectId, teamId]
    );
    await new Promise((resolveDelay) => setTimeout(resolveDelay, 50));
    await first.query("insert into idempotency_test_effects (marker) values ('executed-once')");
    await first.query(
      `update idempotency_records
          set status = 'completed', response_json = '{"ok":true}'::jsonb, completed_at = now()
        where id = $1`,
      [owner.rows[0].id]
    );
    await first.query("commit");

    const contenderResult = await contender;
    assert(contenderResult.rowCount === 0, "concurrent duplicate unexpectedly owned a record");
    const result = await second.query(
      `select record.status, record.response_json as response,
              (select count(*)::int from idempotency_test_effects) as effects
         from idempotency_records record
        where record.project_id = $1 and record.idempotency_key = 'same-key'`,
      [projectId]
    );
    assert(result.rows[0].status === "completed", "duplicate did not observe completed operation");
    assert(result.rows[0].response.ok === true, "duplicate did not observe original response");
    assert(result.rows[0].effects === 1, "concurrent duplicate produced more than one side effect");
  } finally {
    await first.end();
    await second.end();
  }
}

async function assertProjectEventCursorAndDurability(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      "select id as project_id, team_id from projects where name = 'legacy-project'"
    );
    const { project_id: projectId, team_id: teamId } = scope.rows[0];
    const first = await client.query(
      `insert into project_events (project_id, team_id, event_id, event_type)
       values ($1, $2, 'cursor-event-1', 'fixture.cursor') returning cursor`,
      [projectId, teamId]
    );
    const second = await client.query(
      `insert into project_events (project_id, team_id, event_id, event_type)
       values ($1, $2, 'cursor-event-2', 'fixture.cursor') returning cursor`,
      [projectId, teamId]
    );
    assert(
      Number(second.rows[0].cursor) > Number(first.rows[0].cursor),
      "project event cursor did not increase monotonically"
    );

    await client.query(
      `insert into outbox_events (project_id, team_id, event_id, event_type)
       values ($1, $2, 'durable-without-listener', 'fixture.outbox')`,
      [projectId, teamId]
    );
    const claimed = await client.query(
      `update outbox_events
          set status = 'processing', attempts = attempts + 1,
              locked_by = 'fixture-worker', locked_until = now() + interval '30 seconds'
        where event_id = 'durable-without-listener' and status = 'pending'
        returning event_id`
    );
    assert(claimed.rowCount === 1, "outbox event was lost when no notification listener existed");
  } finally {
    await client.end();
  }
}

async function assertDeviceConnectivityConstraints(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const projects = await client.query(
      `select project.id, project.team_id, project.name
         from projects project where project.name in ('北区巡检', '南区巡检')`
    );
    const north = projects.rows.find((project) => project.name === "北区巡检");
    const south = projects.rows.find((project) => project.name === "南区巡检");
    const northAdapter = await client.query(
      `insert into device_adapters (project_id, team_id, name, adapter_type)
       values ($1, $2, 'north-simulator', 'simulator') returning id`,
      [north.id, north.team_id]
    );
    const southAdapter = await client.query(
      `insert into device_adapters (project_id, team_id, name, adapter_type)
       values ($1, $2, 'south-simulator', 'simulator') returning id`,
      [south.id, south.team_id]
    );
    const northDevice = await client.query(
      "select id from devices where project_id = $1 limit 1",
      [north.id]
    );

    await client.query(
      `insert into device_external_identities (
         project_id, team_id, adapter_id, device_id, external_device_id
       ) values ($1, $2, $3, $4, 'cross-project-device')`,
      [north.id, north.team_id, southAdapter.rows[0].id, northDevice.rows[0].id]
    ).then(
      () => assert(false, "cross-project adapter identity binding should fail"),
      (error) => assert(error.code === "23503", "cross-project identity failed unexpectedly")
    );

    await client.query(
      `insert into device_external_identities (
         project_id, team_id, adapter_id, device_id, external_device_id
       ) values ($1, $2, $3, $4, 'north-device')`,
      [north.id, north.team_id, northAdapter.rows[0].id, northDevice.rows[0].id]
    );

    const repeated = await client.query(
      `insert into device_external_identities (
         project_id, team_id, adapter_id, external_device_id, external_device_type, identity_json
       ) values ($1, $2, $3, 'repeated-discovery', 'drone', '{"capabilities":["flight.route"]}'::jsonb)
       on conflict (adapter_id, external_device_id) do update
         set identity_json = excluded.identity_json, last_seen_at = now()
       returning id`,
      [north.id, north.team_id, northAdapter.rows[0].id]
    );
    const repeatedAgain = await client.query(
      `insert into device_external_identities (
         project_id, team_id, adapter_id, external_device_id, external_device_type, identity_json
       ) values ($1, $2, $3, 'repeated-discovery', 'drone', '{"capabilities":["flight.route","camera.live"]}'::jsonb)
       on conflict (adapter_id, external_device_id) do update
         set identity_json = excluded.identity_json, last_seen_at = now()
       returning id`,
      [north.id, north.team_id, northAdapter.rows[0].id]
    );
    assert(repeated.rows[0].id === repeatedAgain.rows[0].id, "repeat discovery created another identity");
    await client.query(
      "update device_external_identities set device_id = $2 where id = $1",
      [repeated.rows[0].id, northDevice.rows[0].id]
    ).then(
      () => assert(false, "conflicting identity binding should fail"),
      (error) => assert(error.code === "23505", "identity binding conflict failed unexpectedly")
    );

    await client.query(
      `insert into device_capabilities (device_id, project_id, capability_code, declared_by_adapter_id)
       values ($1, $2, 'camera.live', $3)`,
      [northDevice.rows[0].id, north.id, northAdapter.rows[0].id]
    );
    const changed = await client.query(
      `insert into device_capabilities (device_id, project_id, capability_code, declared_by_adapter_id)
       values ($1, $2, 'camera.live', $3)
       on conflict (device_id, capability_code) do update
         set version_number = device_capabilities.version_number + 1, updated_at = now()
       returning version_number`,
      [northDevice.rows[0].id, north.id, northAdapter.rows[0].id]
    );
    assert(changed.rows[0].version_number === 2, "capability declaration version did not advance");
  } finally {
    await client.end();
  }
}

async function assertTelemetryIngestionSemantics(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select device.id as device_id, device.project_id, project.team_id, adapter.id as adapter_id
         from devices device
         join projects project on project.id = device.project_id
         join device_adapters adapter on adapter.project_id = project.id
        where project.name = '北区巡检' limit 1`
    );
    const row = scope.rows[0];
    const captured = new Date("2026-08-24T10:00:00.000Z");
    await client.query(
      `create table if not exists device_telemetry_202608
       partition of device_telemetry for values from ('2026-08-01T00:00:00Z') to ('2026-09-01T00:00:00Z')`
    );
    const firstDedup = await client.query(
      `insert into telemetry_event_dedup (adapter_id, event_id, project_id, captured_at)
       values ($1, 'telemetry-event-1', $2, $3)
       on conflict (adapter_id, event_id) do nothing returning event_id`,
      [row.adapter_id, row.project_id, captured]
    );
    const duplicateDedup = await client.query(
      `insert into telemetry_event_dedup (adapter_id, event_id, project_id, captured_at)
       values ($1, 'telemetry-event-1', $2, $3)
       on conflict (adapter_id, event_id) do nothing returning event_id`,
      [row.adapter_id, row.project_id, captured]
    );
    assert(firstDedup.rowCount === 1 && duplicateDedup.rowCount === 0, "telemetry event dedup failed");

    await client.query(
      `insert into device_telemetry (
         project_id, team_id, adapter_id, device_id, event_id, telemetry_type,
         sequence_number, captured_at, payload_json
       ) values ($1, $2, $3, $4, 'telemetry-event-1', 'pose', 2, $5, '{"battery":88}'::jsonb)`,
      [row.project_id, row.team_id, row.adapter_id, row.device_id, captured]
    );
    await client.query(
      `insert into device_latest_telemetry (
         device_id, project_id, adapter_id, event_id, telemetry_type, sequence_number,
         captured_at, received_at, payload_json
       ) values ($1, $2, $3, 'telemetry-event-1', 'pose', 2, $4, now(), '{"battery":88}'::jsonb)`,
      [row.device_id, row.project_id, row.adapter_id, captured]
    );
    await client.query(
      `insert into device_latest_telemetry (
         device_id, project_id, adapter_id, event_id, telemetry_type, sequence_number,
         captured_at, received_at, payload_json
       ) values ($1, $2, $3, 'older-event', 'pose', 99, $4, now(), '{"battery":10}'::jsonb)
       on conflict (device_id) do update set
         event_id = excluded.event_id, sequence_number = excluded.sequence_number,
         captured_at = excluded.captured_at, payload_json = excluded.payload_json
       where excluded.captured_at > device_latest_telemetry.captured_at
          or (excluded.captured_at = device_latest_telemetry.captured_at
              and coalesce(excluded.sequence_number, -1) > coalesce(device_latest_telemetry.sequence_number, -1))`,
      [row.device_id, row.project_id, row.adapter_id, new Date("2026-08-24T09:00:00.000Z")]
    );
    const latest = await client.query(
      "select event_id, payload_json from device_latest_telemetry where device_id = $1",
      [row.device_id]
    );
    assert(latest.rows[0].event_id === "telemetry-event-1", "late telemetry regressed the latest projection");
    const partition = await client.query(
      "select tableoid::regclass::text as partition from device_telemetry where event_id = 'telemetry-event-1'"
    );
    assert(partition.rows[0].partition === "device_telemetry_202608", "telemetry missed its monthly partition");
  } finally {
    await client.end();
  }
}

async function assertHeartbeatProjectionSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select device.id as device_id, device.project_id, project.team_id, adapter.id as adapter_id
         from devices device
         join projects project on project.id = device.project_id
         join device_adapters adapter on adapter.project_id = project.id
        where project.name = '北区巡检' limit 1`
    );
    const row = scope.rows[0];
    await client.query(
      `insert into device_connections (
         project_id, team_id, adapter_id, device_id, session_key, status,
         status_reason, last_heartbeat_at, heartbeat_interval_seconds, status_projected_at
       ) values ($1, $2, $3, $4, 'heartbeat-schema-test', 'online',
                 'heartbeat_fresh', now(), 30, now())`,
      [row.project_id, row.team_id, row.adapter_id, row.device_id]
    );
    await client.query(
      `update device_connections set heartbeat_interval_seconds = 1
        where adapter_id = $1 and session_key = 'heartbeat-schema-test'`,
      [row.adapter_id]
    ).then(
      () => assert(false, "invalid heartbeat interval should fail"),
      (error) => assert(error.code === "23514", "heartbeat interval failed unexpectedly")
    );
    await client.query(
      "update devices set status = 'connected' where id = $1 and project_id = $2",
      [row.device_id, row.project_id]
    ).then(
      () => assert(false, "non-canonical device connectivity status should fail"),
      (error) => assert(error.code === "23514", "device status constraint failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertSpatiotemporalSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select device.id as device_id, device.project_id, project.team_id, adapter.id as adapter_id
         from devices device
         join projects project on project.id = device.project_id
         join device_adapters adapter on adapter.project_id = project.id
        where project.name = '北区巡检' limit 1`
    );
    const row = scope.rows[0];
    const crs = await client.query(
      `insert into coordinate_references (
         project_id, team_id, code, name, authority, vertical_datum, is_project_standard
       ) values ($1, $2, 'EPSG:4326', 'WGS 84', 'EPSG', 'ellipsoid', true) returning id`,
      [row.project_id, row.team_id]
    );
    const calibration = await client.query(
      `insert into sensor_calibrations (
         project_id, team_id, device_id, sensor_key, version, valid_from
       ) values ($1, $2, $3, 'camera.main', 1, '2026-01-01T00:00:00Z') returning id`,
      [row.project_id, row.team_id, row.device_id]
    );
    const observation = await client.query(
      `insert into observations (
         project_id, team_id, adapter_id, device_id, calibration_id,
         observation_type, source_event_id, captured_at, received_at,
         original_crs_id, original_geometry, standard_geometry, quality_json
       ) values (
         $1, $2, $3, $4, $5, 'pose', 'observation-pose-1',
         '2026-08-24T10:00:00Z', '2026-08-24T10:00:01Z', $6,
         ST_SetSRID(ST_MakePoint(120.1, 30.2, 88), 4326),
         ST_SetSRID(ST_MakePoint(120.1, 30.2, 88), 4326),
         '{"horizontalAccuracyMeters":1.2}'::jsonb
       ) returning id`,
      [row.project_id, row.team_id, row.adapter_id, row.device_id, calibration.rows[0].id, crs.rows[0].id]
    );
    await client.query(
      `insert into poses (
         observation_id, project_id, device_id, captured_at, standard_position,
         original_position, orientation_w, horizontal_accuracy_m, vertical_datum, transform_version
       ) values (
         $1, $2, $3, '2026-08-24T10:00:00Z',
         ST_SetSRID(ST_MakePoint(120.1, 30.2, 88), 4326),
         ST_SetSRID(ST_MakePoint(120.1, 30.2, 88), 4326),
         1, 1.2, 'ellipsoid', '1'
       )`,
      [observation.rows[0].id, row.project_id, row.device_id]
    );
    const geometry = await client.query(
      `select ST_SRID(standard_position) as srid, ST_NDims(standard_position) as dimensions,
              horizontal_accuracy_m
         from poses where observation_id = $1`,
      [observation.rows[0].id]
    );
    assert(geometry.rows[0].srid === 4326, "standard pose SRID is not WGS84");
    assert(geometry.rows[0].dimensions === 3, "standard pose did not preserve altitude");
    assert(Number(geometry.rows[0].horizontal_accuracy_m) === 1.2, "pose quality was not preserved");

    const south = await client.query(
      `select project.id, project.team_id, device.id as device_id, adapter.id as adapter_id
         from projects project
         join devices device on device.project_id = project.id
         join device_adapters adapter on adapter.project_id = project.id
        where project.name = '南区巡检' limit 1`
    );
    await client.query(
      `insert into observations (
         project_id, team_id, adapter_id, device_id, calibration_id,
         observation_type, source_event_id, captured_at, received_at
       ) values ($1, $2, $3, $4, $5, 'pose', 'cross-project-calibration', now(), now())`,
      [south.rows[0].id, south.rows[0].team_id, south.rows[0].adapter_id,
       south.rows[0].device_id, calibration.rows[0].id]
    ).then(
      () => assert(false, "cross-project calibration reference should fail"),
      (error) => assert(error.code === "23503", "cross-project calibration failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertMediaEvidenceSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const projects = await client.query(
      `select project.id, project.team_id, project.name, device.id as device_id
         from projects project
         join devices device on device.project_id = project.id
        where project.name in ('北区巡检', '南区巡检')`
    );
    const north = projects.rows.find((project) => project.name === "北区巡检");
    const south = projects.rows.find((project) => project.name === "南区巡检");
    await client.query(
      `insert into asset_upload_intents (
         id, project_id, team_id, logical_key, object_key, file_name, kind, mime_type,
         expected_size_bytes, expected_checksum_sha256, device_id, expires_at
       ) values (
         '00000000-0000-4000-8000-000000000001', $1, $2, 'mission/frame',
         'projects/cross/uploads/forged/frame.jpg', 'frame.jpg', 'image', 'image/jpeg',
         4, repeat('a', 64), $3, now() + interval '15 minutes'
       )`,
      [north.id, north.team_id, south.device_id]
    ).then(
      () => assert(false, "cross-project upload source should fail"),
      (error) => assert(error.code === "23503", "cross-project upload source failed unexpectedly")
    );

    const asset = await client.query(
      `insert into assets (
         project_id, team_id, device_id, kind, mime_type, storage_key, logical_key,
         version, status, size_bytes, checksum_sha256, available_at
       ) values (
         $1, $2, $3, 'image', 'image/jpeg', 'projects/north/uploads/one/frame.jpg',
         'mission/frame', 1, 'available', 4, repeat('a', 64), now()
       ) returning id`,
      [north.id, north.team_id, north.device_id]
    );
    const evidence = await client.query(
      `insert into evidence_links (
         project_id, team_id, target_type, target_id, asset_id,
         asset_version, asset_checksum_sha256, is_published
       ) values ($1, $2, 'report', 'report-1', $3, 1, repeat('a', 64), true)
       returning id`,
      [north.id, north.team_id, asset.rows[0].id]
    );
    await client.query(
      "update evidence_links set asset_version = 2 where id = $1",
      [evidence.rows[0].id]
    ).then(
      () => assert(false, "published evidence should be immutable"),
      (error) => assert(error.code === "55000", "published evidence mutation failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertLiveStreamSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select project.id, project.team_id, project.name, device.id as device_id,
              (select adapter.id from device_adapters adapter
                where adapter.project_id = project.id limit 1) as adapter_id
         from projects project
         join devices device on device.project_id = project.id
        where project.name in ('北区巡检', '南区巡检')`
    );
    const north = scope.rows.find((row) => row.name === "北区巡检" && row.adapter_id);
    const south = scope.rows.find((row) => row.name === "南区巡检");
    const stream = await client.query(
      `insert into live_streams (
         project_id, team_id, device_id, adapter_id, stream_key, source_type, status, playback_ref
       ) values ($1, $2, $3, $4, 'camera.main', 'simulator', 'live', 'simulator://camera.main')
       returning id`,
      [north.id, north.team_id, north.device_id, north.adapter_id]
    );
    await client.query(
      `insert into live_streams (
         project_id, team_id, device_id, adapter_id, stream_key, source_type, status
       ) values ($1, $2, $3, $4, 'camera.main', 'simulator', 'starting')`,
      [north.id, north.team_id, north.device_id, north.adapter_id]
    ).then(
      () => assert(false, "a device stream key should have one active session"),
      (error) => assert(error.code === "23505", "duplicate active stream failed unexpectedly")
    );
    await client.query(
      "update live_streams set device_id = $3 where project_id = $1 and id = $2",
      [north.id, stream.rows[0].id, south.device_id]
    ).then(
      () => assert(false, "cross-project live stream device should fail"),
      (error) => assert(error.code === "23503", "cross-project live stream failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertTaskVersionSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select project.id, project.team_id, actor.id as user_id
         from projects project join users actor on actor.email = 'legacy@example.test'
        where project.name = 'legacy-project'`
    );
    const row = scope.rows[0];
    const task = await client.query(
      `insert into tasks (
         project_id, team_id, name, trigger_type, script, created_by_user_id
       ) values ($1, $2, 'versioned-task', 'manual', 'inspect', $3) returning id`,
      [row.id, row.team_id, row.user_id]
    );
    const version = await client.query(
      `insert into task_versions (
         project_id, team_id, task_id, version, status, definition_json, script, created_by_user_id
       ) values ($1, $2, $3, 1, 'draft', '{"name":"versioned-task"}'::jsonb, 'inspect', $4)
       returning id`,
      [row.id, row.team_id, task.rows[0].id, row.user_id]
    );
    await client.query(
      `insert into task_steps (
         project_id, team_id, task_version_id, position, step_key, name, action
       ) values ($1, $2, $3, 1, 'capture', '采集', 'camera.capture')`,
      [row.id, row.team_id, version.rows[0].id]
    );
    await client.query(
      `update task_versions set status = 'published', published_by_user_id = $2, published_at = now()
        where id = $1`,
      [version.rows[0].id, row.user_id]
    );
    await client.query(
      "update task_versions set script = 'changed' where id = $1",
      [version.rows[0].id]
    ).then(
      () => assert(false, "published task version should be immutable"),
      (error) => assert(error.code === "55000", "published task mutation failed unexpectedly")
    );
    await client.query(
      "update task_steps set action = 'changed' where task_version_id = $1",
      [version.rows[0].id]
    ).then(
      () => assert(false, "published task steps should be immutable"),
      (error) => assert(error.code === "55000", "published task step mutation failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertSafetyPolicySchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select project.id, project.team_id, actor.id as user_id
         from projects project join users actor on actor.email = 'legacy@example.test'
        where project.name = 'legacy-project'`
    );
    const row = scope.rows[0];
    const policy = await client.query(
      `insert into safety_policy_versions (
         project_id, team_id, version, status, project_boundary, restricted_areas,
         max_altitude_meters, max_speed_meters_per_second, minimum_battery_percent,
         required_compliance_json, published_by_user_id, published_at
       ) values (
         $1, $2, 1, 'published',
         ST_GeomFromText('POLYGON((120 30,122 30,122 32,120 32,120 30))', 4326),
         ST_GeomFromText('MULTIPOLYGON(((120.8 30.8,121.2 30.8,121.2 31.2,120.8 31.2,120.8 30.8)))', 4326),
         120, 15, 30, '["flightApproval"]'::jsonb, $3, now()
       ) returning id`,
      [row.id, row.team_id, row.user_id]
    );
    await client.query(
      "update projects set current_safety_policy_version_id = $2 where id = $1",
      [row.id, policy.rows[0].id]
    );
    const spatial = await client.query(
      `select ST_Covers(project_boundary, ST_SetSRID(ST_MakePoint(121, 31), 4326)) as in_boundary,
              ST_Intersects(restricted_areas, ST_SetSRID(ST_MakePoint(121, 31), 4326)) as restricted
         from safety_policy_versions where id = $1`,
      [policy.rows[0].id]
    );
    assert(spatial.rows[0].in_boundary && spatial.rows[0].restricted, "safety policy geometry is not queryable");
    await client.query(
      "update safety_policy_versions set max_altitude_meters = 200 where id = $1",
      [policy.rows[0].id]
    ).then(
      () => assert(false, "published safety policy should be immutable"),
      (error) => assert(error.code === "55000", "published safety policy mutation failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertApprovalSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select project.id, project.team_id, actor.id as user_id
         from projects project join users actor on actor.email = 'legacy@example.test'
        where project.name = 'legacy-project'`
    );
    const row = scope.rows[0];
    const approver = await client.query(
      "insert into users (name, email) values ('approver', 'approver@example.test') returning id"
    );
    await client.query(
      "insert into team_members (team_id, user_id, role) values ($1, $2, 'admin')",
      [row.team_id, approver.rows[0].id]
    );
    await client.query(
      `insert into approval_requests (
         id, project_id, team_id, resource_type, resource_id, action,
         requested_by_user_id, required_approvals, expires_at
       ) values ('00000000-0000-4000-8000-000000000001', $1, $2, 'task_run', '1', 'start', $3, 2, now() + interval '1 hour')`,
      [row.id, row.team_id, row.user_id]
    );
    await client.query(
      `insert into approvals (project_id, team_id, approval_request_id, approver_user_id, decision, reason)
       values ($1, $2, '00000000-0000-4000-8000-000000000001', $3, 'approved', 'self')`,
      [row.id, row.team_id, row.user_id]
    ).then(
      () => assert(false, "requester self-approval should fail"),
      (error) => assert(error.code === "42501", "self-approval failed unexpectedly")
    );
    await client.query(
      `insert into approvals (project_id, team_id, approval_request_id, approver_user_id, decision, reason)
       values ($1, $2, '00000000-0000-4000-8000-000000000001', $3, 'approved', 'reviewed')`,
      [row.id, row.team_id, approver.rows[0].id]
    );
    await client.query(
      `insert into approvals (project_id, team_id, approval_request_id, approver_user_id, decision, reason)
       values ($1, $2, '00000000-0000-4000-8000-000000000001', $3, 'approved', 'duplicate')`,
      [row.id, row.team_id, approver.rows[0].id]
    ).then(
      () => assert(false, "duplicate approver should fail"),
      (error) => assert(error.code === "23505", "duplicate approval failed unexpectedly")
    );
    await client.query(
      `insert into approval_requests (
         id, project_id, team_id, resource_type, resource_id, action,
         requested_by_user_id, expires_at
       ) values ('00000000-0000-4000-8000-000000000002', $1, $2, 'task_run', '2', 'start', $3, now() - interval '1 hour')`,
      [row.id, row.team_id, row.user_id]
    );
    await client.query(
      `insert into approvals (project_id, team_id, approval_request_id, approver_user_id, decision, reason)
       values ($1, $2, '00000000-0000-4000-8000-000000000002', $3, 'approved', 'late')`,
      [row.id, row.team_id, approver.rows[0].id]
    ).then(
      () => assert(false, "expired approval should fail"),
      (error) => assert(error.code === "55000", "expired approval failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertTaskRunCommandSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = await client.query(
      `select project.id, project.team_id, actor.id as user_id,
              task.id as task_id, version.id as version_id, step.id as step_id
         from projects project
         join users actor on actor.email = 'legacy@example.test'
         join tasks task on task.project_id = project.id and task.name = 'versioned-task'
         join task_versions version on version.task_id = task.id
         join task_steps step on step.task_version_id = version.id
        where project.name = 'legacy-project' limit 1`
    );
    const row = scope.rows[0];
    const adapter = await client.query(
      `insert into device_adapters (project_id, team_id, name, adapter_type)
       values ($1, $2, 'mission-simulator', 'simulator') returning id`,
      [row.id, row.team_id]
    );
    const device = await client.query(
      `insert into devices (project_id, name, type, adapter_id, status)
       values ($1, 'mission-drone', 'drone', $2, 'online') returning id`,
      [row.id, adapter.rows[0].id]
    );
    const run = await client.query(
      `insert into task_runs (
         project_id, team_id, task_id, task_version_id, selected_device_id, trigger_source, created_by_user_id
       ) values ($1, $2, $3, $4, $5, 'manual', $6) returning id, state_version`,
      [row.id, row.team_id, row.task_id, row.version_id, device.rows[0].id, row.user_id]
    );
    const runStep = await client.query(
      `insert into task_run_steps (project_id, team_id, task_run_id, task_step_id, position)
       values ($1, $2, $3, $4, 1) returning id`,
      [row.id, row.team_id, run.rows[0].id, row.step_id]
    );
    await client.query(
      `insert into device_commands (
         id, project_id, team_id, task_run_id, task_run_step_id, device_id,
         command_key, idempotency_key, capability_code, status, deadline_at, requested_by_user_id
       ) values ('10000000-0000-4000-8000-000000000001', $1, $2, $3, $4, $5,
                 'step-1', 'run-step-1', 'camera.capture', 'sent', now() + interval '1 minute', $6)`,
      [row.id, row.team_id, run.rows[0].id, runStep.rows[0].id, device.rows[0].id, row.user_id]
    );
    await client.query(
      `insert into command_attempts (project_id, team_id, command_id, adapter_id, attempt, status)
       values ($1, $2, '10000000-0000-4000-8000-000000000001', $3, 1, 'sent')`,
      [row.id, row.team_id, adapter.rows[0].id]
    );
    await client.query(
      `insert into device_commands (
         id, project_id, team_id, task_run_id, device_id, command_key, idempotency_key,
         capability_code, deadline_at
       ) values ('10000000-0000-4000-8000-000000000002', $1, $2, $3, $4,
                 'duplicate', 'run-step-1', 'camera.capture', now() + interval '1 minute')`,
      [row.id, row.team_id, run.rows[0].id, device.rows[0].id]
    ).then(
      () => assert(false, "duplicate physical command idempotency key should fail"),
      (error) => assert(error.code === "23505", "duplicate command failed unexpectedly")
    );
    const advanced = await client.query(
      `update task_runs set status = 'ready', state_version = state_version + 1, state_reason = 'preflight passed'
        where id = $1 and project_id = $2 and state_version = 0 returning state_version`,
      [run.rows[0].id, row.id]
    );
    const stale = await client.query(
      `update task_runs set status = 'dispatching', state_version = state_version + 1
        where id = $1 and project_id = $2 and state_version = 0 returning state_version`,
      [run.rows[0].id, row.id]
    );
    assert(advanced.rows[0].state_version === 1 && stale.rowCount === 0, "task run optimistic version did not prevent stale update");
  } finally {
    await client.end();
  }
}

async function assertAlgorithmRuntimeSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const projects = await client.query(
      `select project.id, project.team_id, project.name, actor.id as user_id
         from projects project
         join team_members member on member.team_id = project.team_id and member.role = 'owner'
         join users actor on actor.id = member.user_id
        where project.name in ('北区巡检', '南区巡检')`
    );
    const north = projects.rows.find((row) => row.name === "北区巡检");
    const south = projects.rows.find((row) => row.name === "南区巡检");
    const asset = await client.query("select id from assets where project_id = $1 and status = 'available' limit 1", [north.id]);
    const provider = await client.query(
      `insert into algorithm_providers (
         project_id, team_id, name, provider_type, base_url, created_by_user_id
       ) values ($1, $2, 'fixture-http', 'http-json', 'https://algorithm.example.test', $3) returning id`,
      [north.id, north.team_id, north.user_id]
    );
    await client.query(
      `insert into algorithm_definitions (project_id, team_id, provider_id, name, capability_code)
       values ($1, $2, $3, 'cross-project', 'vision.detect')`,
      [south.id, south.team_id, provider.rows[0].id]
    ).then(
      () => assert(false, "cross-project algorithm provider binding should fail"),
      (error) => assert(error.code === "23503", "cross-project algorithm binding failed unexpectedly")
    );
    const definition = await client.query(
      `insert into algorithm_definitions (
         project_id, team_id, provider_id, name, capability_code, created_by_user_id
       ) values ($1, $2, $3, 'fixture-detection', 'vision.detect', $4) returning id`,
      [north.id, north.team_id, provider.rows[0].id, north.user_id]
    );
    const version = await client.query(
      `insert into algorithm_definition_versions (
         project_id, team_id, algorithm_definition_id, version, status, execution_mode,
         model_or_process, output_mapping_json, created_by_user_id
       ) values ($1, $2, $3, 1, 'draft', 'synchronous', 'model-v1',
                 '{"label":"$.label"}'::jsonb, $4) returning id`,
      [north.id, north.team_id, definition.rows[0].id, north.user_id]
    );
    await client.query(
      `update algorithm_definition_versions set status = 'published', published_by_user_id = $2, published_at = now()
        where id = $1`, [version.rows[0].id, north.user_id]
    );
    await client.query(
      "update algorithm_definition_versions set model_or_process = 'changed' where id = $1",
      [version.rows[0].id]
    ).then(
      () => assert(false, "published algorithm definition version should be immutable"),
      (error) => assert(error.code === "55000", "published algorithm mutation failed unexpectedly")
    );
    await client.query(
      `insert into algorithm_runs (
         id, project_id, team_id, algorithm_definition_version_id, input_asset_id, idempotency_key
       ) values ('20000000-0000-4000-8000-000000000001', $1, $2, $3, $4, 'asset:fixture:v1')`,
      [north.id, north.team_id, version.rows[0].id, asset.rows[0].id]
    );
    await client.query(
      `insert into algorithm_runs (
         id, project_id, team_id, algorithm_definition_version_id, input_asset_id, idempotency_key
       ) values ('20000000-0000-4000-8000-000000000002', $1, $2, $3, $4, 'asset:fixture:v1')`,
      [north.id, north.team_id, version.rows[0].id, asset.rows[0].id]
    ).then(
      () => assert(false, "duplicate algorithm run idempotency key should fail"),
      (error) => assert(error.code === "23505", "duplicate algorithm run failed unexpectedly")
    );
    await client.query(
      `insert into algorithm_callback_receipts (
         project_id, team_id, algorithm_run_id, provider_id, callback_id, external_job_id, payload_hash
       ) values ($1, $2, '20000000-0000-4000-8000-000000000001', $3, 'callback-1', 'job-1', $4)`,
      [north.id, north.team_id, provider.rows[0].id, "a".repeat(64)]
    );
    await client.query(
      `insert into algorithm_callback_receipts (
         project_id, team_id, algorithm_run_id, provider_id, callback_id, external_job_id, payload_hash
       ) values ($1, $2, '20000000-0000-4000-8000-000000000001', $3, 'callback-1', 'job-1', $4)`,
      [north.id, north.team_id, provider.rows[0].id, "a".repeat(64)]
    ).then(
      () => assert(false, "replayed provider callback id should fail"),
      (error) => assert(error.code === "23505", "callback replay constraint failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

async function assertDetectionSchema(connectionString) {
  const client = new Client({ connectionString });
  await client.connect();
  try {
    const scope = (await client.query(
      `select run.project_id, run.team_id, run.input_asset_id, run.task_run_id
         from algorithm_runs run where run.id='20000000-0000-4000-8000-000000000001'`
    )).rows[0];
    const detection = await client.query(
      `insert into detections (
         project_id,team_id,algorithm_run_id,input_asset_id,task_run_id,detection_key,label,confidence,
         pixel_geometry_json,geographic_geometry,location_quality,projection_method,horizontal_error_meters,
         transform_version,captured_at
       ) values ($1,$2,'20000000-0000-4000-8000-000000000001',$3,$4,'d-1','suspected-construction',0.9,
         '{"type":"bbox","x":1,"y":2,"width":3,"height":4}',
         st_geomfromtext('POLYGON((120 30,120.001 30,120.001 30.001,120 30.001,120 30))',4326),
         'estimated','nadir-ray-ground-plane',2.5,'aerosight-geo-projection/v1',now()) returning id`,
      [scope.project_id, scope.team_id, scope.input_asset_id, scope.task_run_id]
    );
    const group = await client.query(
      `insert into detection_groups (project_id,team_id,label,location_quality,first_detected_at,last_detected_at)
       values ($1,$2,'suspected-construction','estimated',now(),now()) returning id`,
      [scope.project_id, scope.team_id]
    );
    await client.query(
      `insert into detection_group_members (project_id,team_id,detection_group_id,detection_id) values ($1,$2,$3,$4)`,
      [scope.project_id, scope.team_id, group.rows[0].id, detection.rows[0].id]
    );
    await client.query(
      `insert into detection_group_members (project_id,team_id,detection_group_id,detection_id) values ($1,$2,$3,$4)`,
      [scope.project_id, scope.team_id, group.rows[0].id, detection.rows[0].id]
    ).then(
      () => assert(false, "detection should belong to only one group"),
      (error) => assert(error.code === "23505", "detection group uniqueness failed unexpectedly")
    );
  } finally {
    await client.end();
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

const unavailableClient = { query: async () => ({ rowCount: 0, rows: [] }) };
await assertPostgisAvailable(unavailableClient).then(
  () => assert(false, "missing PostGIS should fail availability validation"),
  (error) => assert(String(error).includes("PostGIS extension is required"), "unclear PostGIS error")
);

const testPostgis = await startTestPostgis();
adminUrl = testPostgis.url;
try {
  await withTemporaryDatabase("empty", async (connectionString) => {
    const first = await migrateDatabase({ connectionString, logger: silentLogger });
    const state = await readMigrationState(connectionString);
    assert(first.applied.length === 21, "empty database should apply all migrations");
    assert(first.applied[0].adopted === false, "empty database baseline must execute, not adopt");
    assert(state.tables.users && state.tables.projects && state.tables.devices, "baseline tables missing");
    assert(state.tables.postgis_version, "PostGIS version was not queryable");
    assert(state.tables.point_z === "POINT Z (120.1 30.2 88)", "PointZ round trip failed");
    await assertLegacyProjectDefaults(connectionString);
    await assertProjectIsolationFixture(connectionString);
    await assertConcurrentIdempotency(connectionString);
    await assertProjectEventCursorAndDurability(connectionString);
    await assertDeviceConnectivityConstraints(connectionString);
    await assertTelemetryIngestionSemantics(connectionString);
    await assertHeartbeatProjectionSchema(connectionString);
    await assertSpatiotemporalSchema(connectionString);
    await assertMediaEvidenceSchema(connectionString);
    await assertLiveStreamSchema(connectionString);
    await assertTaskVersionSchema(connectionString);
    await assertSafetyPolicySchema(connectionString);
    await assertApprovalSchema(connectionString);
    await assertTaskRunCommandSchema(connectionString);
    await assertAlgorithmRuntimeSchema(connectionString);
    await assertDetectionSchema(connectionString);

    const second = await migrateDatabase({ connectionString, logger: silentLogger });
    assert(second.applied.length === 0, "second empty-database migration run must be a no-op");
  });

  await withTemporaryDatabase("existing", async (connectionString) => {
    const legacy = new Client({ connectionString });
    await legacy.connect();
    await legacy.query(legacySchema);
    const legacyUser = await legacy.query("insert into users (name, email) values ('upgrade-user', 'upgrade@example.test') returning id");
    const legacyTeam = await legacy.query("insert into teams (name) values ('upgrade-team') returning id");
    await legacy.query("insert into team_members (team_id, user_id, role) values ($1, $2, 'owner')", [legacyTeam.rows[0].id, legacyUser.rows[0].id]);
    const legacyProject = await legacy.query("insert into projects (team_id, name) values ($1, 'upgrade-project') returning id", [legacyTeam.rows[0].id]);
    const legacyTask = await legacy.query(
      "insert into tasks (project_id, name, trigger_type, script, created_by_user_id) values ($1, 'legacy-task', 'manual', 'legacy-script', $2) returning id",
      [legacyProject.rows[0].id, legacyUser.rows[0].id]
    );
    await legacy.query(
      "insert into task_runs (project_id, task_id, trigger_source, created_by_user_id) values ($1, $2, 'manual', $3)",
      [legacyProject.rows[0].id, legacyTask.rows[0].id, legacyUser.rows[0].id]
    );
    await legacy.end();

    const first = await migrateDatabase({ connectionString, logger: silentLogger });
    const before = await readMigrationState(connectionString);
    assert(first.applied.length === 21, "existing database should record all migrations");
    assert(first.applied[0].adopted === true, "existing database should adopt the baseline");
    const upgraded = new Client({ connectionString });
    await upgraded.connect();
    const upgradedTask = await upgraded.query(
      `select task.current_published_version_id, version.status, version.script,
              run.task_version_id
         from tasks task
         join task_versions version on version.id = task.current_published_version_id
         join task_runs run on run.task_id = task.id and run.project_id = task.project_id
        where task.name = 'legacy-task'`
    );
    await upgraded.end();
    assert(upgradedTask.rows[0]?.status === "published", "legacy task did not receive a published compatibility version");
    assert(upgradedTask.rows[0]?.script === "legacy-script", "legacy task script changed during upgrade");
    assert(upgradedTask.rows[0]?.task_version_id === upgradedTask.rows[0]?.current_published_version_id, "legacy run did not pin its compatibility version");

    const second = await migrateDatabase({ connectionString, logger: silentLogger });
    const after = await readMigrationState(connectionString);
    assert(second.applied.length === 0, "second existing-database migration run must be a no-op");
    assert(JSON.stringify(before) === JSON.stringify(after), "second run changed migration state");
  });

  await withTemporaryDatabase("snapshot", async (connectionString) => {
    const client = new Client({ connectionString });
    await client.connect();
    try {
      await client.query(currentSchema);
      const result = await client.query(
        `select to_regclass('public.users') as users,
                to_regclass('public.device_adapters') as adapters,
                to_regclass('public.device_telemetry') as telemetry,
                to_regclass('public.asset_upload_intents') as upload_intents,
                to_regclass('public.evidence_links') as evidence_links,
                to_regclass('public.live_streams') as live_streams,
                to_regclass('public.task_versions') as task_versions,
                to_regclass('public.task_steps') as task_steps,
                to_regclass('public.safety_policy_versions') as safety_policy_versions,
                to_regclass('public.approvals') as approvals,
                to_regclass('public.task_run_steps') as task_run_steps,
                to_regclass('public.device_commands') as device_commands,
                to_regclass('public.command_attempts') as command_attempts,
                to_regclass('public.algorithm_providers') as algorithm_providers,
                to_regclass('public.algorithm_definitions') as algorithm_definitions,
                to_regclass('public.algorithm_definition_versions') as algorithm_definition_versions,
                to_regclass('public.algorithm_runs') as algorithm_runs,
                to_regclass('public.algorithm_run_attempts') as algorithm_run_attempts,
                to_regclass('public.detections') as detections,
                to_regclass('public.detection_groups') as detection_groups,
                to_regclass('public.detection_group_members') as detection_group_members`
      );
      assert(
        result.rows[0].users && result.rows[0].adapters && result.rows[0].telemetry &&
        result.rows[0].upload_intents && result.rows[0].evidence_links && result.rows[0].live_streams &&
        result.rows[0].task_versions && result.rows[0].task_steps && result.rows[0].safety_policy_versions &&
        result.rows[0].approvals && result.rows[0].task_run_steps && result.rows[0].device_commands &&
        result.rows[0].command_attempts && result.rows[0].algorithm_providers &&
        result.rows[0].algorithm_definitions && result.rows[0].algorithm_definition_versions &&
        result.rows[0].algorithm_runs && result.rows[0].algorithm_run_attempts &&
        result.rows[0].detections && result.rows[0].detection_groups && result.rows[0].detection_group_members,
        "schema snapshot is incomplete"
      );
    } finally {
      await client.end();
    }
  });
} finally {
  testPostgis.cleanup();
}

console.info("Database migration tests passed: PostGIS, empty, existing, and repeated execution.");
