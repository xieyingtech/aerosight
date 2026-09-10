import { randomBytes } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readdir, readFile, mkdir, copyFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { createRequire } from 'node:module';
import { verifyLegacyServices } from './verify-legacy-services.mjs';
import { verifyGoCutover } from './verify-go-cutover.mjs';

import pg from "pg";

import { migrateDatabase } from "./db-migrate.mjs";

const { Client } = pg;
const databaseImage = process.env.ROLLBACK_DRILL_POSTGIS_IMAGE ?? "postgis/postgis:17-3.5";
const migrationsDirectory = resolve(import.meta.dirname, "../../db/migrations");
const legacySchema = await readFile(resolve(migrationsDirectory, "0001_baseline.sql"), "utf8");
const migrationCount = (await readdir(migrationsDirectory))
  .filter((name) => /^\d{4}_[a-z0-9_]+\.sql$/.test(name))
  .length;
const silentLogger = { info() {} };
const root = resolve(import.meta.dirname, '../..');
const output = resolve(root, '.build', `upgrade-rollback-${randomBytes(8).toString('hex')}`);
await mkdir(output, { recursive: true });
const executable = resolve(output, process.platform === 'win32' ? 'aerosight.exe' : 'aerosight');
await copyFile(resolve(root, '.build', process.platform === 'win32' ? 'aerosight.exe' : 'aerosight'), executable);
const legacyBoundary = '0051_connector_external_scope_key.sql';
const args = process.argv.slice(2);
assert(args.length === 0 || (args.length === 2 && args[0] === '--legacy-release'), 'usage: upgrade-rollback-drill.mjs [--legacy-release path]');
const release = args[1] ? resolve(args[1]) : null;
const password = randomBytes(16).toString('hex');
const secret = randomBytes(32).toString('hex');
const passwordHash = release ? await createRequire(resolve(release, 'apps/web/package.json'))('bcryptjs').hash(password, 10) : null;

async function migrateWithGo(url, label) {
  const result = spawnSync(executable, ['migrate'], {
    cwd: output, encoding: 'utf8', timeout: 60000, env: { ...process.env, DATABASE_URL: url },
  });
  await writeFile(resolve(output, `${label}.log`), `${result.stdout ?? ''}${result.stderr ?? ''}`);
  assert(result.status === 0, `Go migration failed; inspect ${output}: ${result.error ?? result.stderr}`);
  const completion = result.stdout.trim().split(/\r?\n/).map(line => JSON.parse(line)).find(line => line.msg === 'migrations complete');
  assert(completion && Number.isInteger(completion.applied), 'Go migration completion missing');
  return completion.applied;
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function docker(...args) {
  const result = spawnSync("docker", args, { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr.trim() || `docker ${args[0]} failed`);
  return result.stdout.trim();
}

async function startPostgis() {
  const name = `aerosight-rollback-drill-${randomBytes(5).toString("hex")}`;
  docker("run", "--detach", "--rm", "--name", name, "--env", "POSTGRES_PASSWORD=aerosight-rollback",
    "--publish", "127.0.0.1::5432", databaseImage);
  try {
    const port = docker("port", name, "5432/tcp").match(/:(\d+)$/)?.[1];
    if (!port) throw new Error("Could not determine rollback drill PostgreSQL port");
    const url = `postgresql://postgres:aerosight-rollback@127.0.0.1:${port}/postgres`;
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
    throw lastError ?? new Error("Timed out waiting for rollback drill PostGIS");
  } catch (error) {
    docker("stop", name);
    throw error;
  }
}

async function seedLegacySnapshot(client) {
  await client.query(legacySchema);
  const user = (await client.query(
    "insert into users(name,email) values('rollback-owner','rollback@example.test') returning id"
  )).rows[0];
  const team = (await client.query("insert into teams(name) values('rollback-team') returning id")).rows[0];
  await client.query("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", [team.id, user.id]);
  const project = (await client.query(
    "insert into projects(team_id,name,description,created_by_user_id) values($1,'rollback-project','legacy snapshot',$2) returning id",
    [team.id, user.id]
  )).rows[0];
  const device = (await client.query(
    "insert into devices(project_id,name,type,status) values($1,'legacy-drone','drone','online') returning id",
    [project.id]
  )).rows[0];
  const task = (await client.query(
    `insert into tasks(project_id,name,trigger_type,status,script,created_by_user_id)
     values($1,'legacy-inspection','manual','active','return legacy mission',$2) returning id`,
    [project.id, user.id]
  )).rows[0];
  const run = (await client.query(
    `insert into task_runs(project_id,task_id,trigger_source,status,created_by_user_id)
     values($1,$2,'manual','queued',$3) returning id`, [project.id, task.id, user.id]
  )).rows[0];
  const asset = (await client.query(
    `insert into assets(project_id,device_id,task_run_id,kind,mime_type,storage_key,checksum,captured_at)
     values($1,$2,$3,'image','image/jpeg','legacy/evidence.jpg',$4,now()) returning id`,
    [project.id, device.id, run.id, "c".repeat(64)]
  )).rows[0];
  return { userId: user.id, teamId: team.id, projectId: project.id, deviceId: device.id,
    taskId: task.id, runId: run.id, legacyAssetId: asset.id };
}

async function readLegacyPageContract(client, scope) {
  const project = (await client.query(
    `select project.id,project.name,project.description,membership.role
       from projects project join team_members membership on membership.team_id=project.team_id
      where project.id=$1 and membership.user_id=$2`, [scope.projectId, scope.userId]
  )).rows[0];
  const devices = (await client.query(
    "select id,name,type,status from devices where project_id=$1 order by id", [scope.projectId]
  )).rows;
  const tasks = (await client.query(
    "select id,name,trigger_type,status,script from tasks where project_id=$1 order by id", [scope.projectId]
  )).rows;
  const runs = (await client.query(
    "select id,task_id,trigger_source,status from task_runs where project_id=$1 order by id", [scope.projectId]
  )).rows;
  const assets = (await client.query(
    "select id,device_id,task_run_id,kind,mime_type,storage_key,checksum from assets where project_id=$1 order by id",
    [scope.projectId]
  )).rows;
  return { project, devices, tasks, runs, assets };
}

async function writeNewEvidenceAndFutureEvent(client, scope) {
  const checksum = "d".repeat(64);
  const asset = (await client.query(
    `insert into assets(
       project_id,team_id,device_id,task_run_id,kind,mime_type,storage_key,logical_key,version,status,
       size_bytes,checksum,checksum_sha256,available_at,captured_at,legal_hold,retention_reason
     ) values($1,$2,$3,$4,'image','image/jpeg','projects/rollback/new-evidence.jpg','rollback/new-evidence.jpg',
       1,'available',128,$5,$5,now(),now(),true,'rollback drill preservation') returning id`,
    [scope.projectId, scope.teamId, scope.deviceId, scope.runId, checksum]
  )).rows[0];
  await client.query(
    `insert into evidence_links(
       project_id,team_id,target_type,target_id,asset_id,asset_version,asset_checksum_sha256,is_published,created_by_user_id
     ) values($1,$2,'task_run',$3,$4,1,$5,true,$6)`,
    [scope.projectId, scope.teamId, String(scope.runId), asset.id, checksum, scope.userId]
  );
  await client.query(
    `insert into retention_holds(project_id,team_id,asset_id,reason,created_by_user_id)
     values($1,$2,$3,'preserve evidence across application rollback',$4)`,
    [scope.projectId, scope.teamId, asset.id, scope.userId]
  );
  await client.query(
    `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
     values($1,$2,'future-evidence-event','future.evidence.sealed',$3)`,
    [scope.projectId, scope.teamId, { assetId: asset.id }]
  );
  return asset.id;
}

async function simulateRollbackWorkerClaim(client) {
  const supportedByRollbackWorker = [
    "task_run.transitioned", "mission.control", "command.ack", "asset.available",
    "algorithm.run.requested", "alert.automation.requested"
  ];
  const claimed = await client.query(
    `with candidates as (
       select id from outbox_events
        where attempts < max_attempts and event_type=any($1::text[]) and available_at<=now()
          and (status='pending' or (status='processing' and locked_until<now()))
        order by available_at,id for update skip locked limit 20
     )
     update outbox_events event set status='processing',attempts=event.attempts+1,
       locked_by='rollback-worker',locked_until=now()+interval '30 seconds'
     from candidates where event.id=candidates.id returning event.event_id`,
    [supportedByRollbackWorker]
  );
  assert(claimed.rowCount === 0, "rollback worker unexpectedly claimed a future event");
}

const postgis = await startPostgis();
const client = new Client({ connectionString: postgis.url });
try {
  await client.connect();
  const scope = await seedLegacySnapshot(client);
  if (passwordHash) await client.query('update users set password=$1 where id=$2', [passwordHash, scope.userId]);
  const beforeUpgrade = await readLegacyPageContract(client, scope);
  assert(beforeUpgrade.project?.name === "rollback-project", "legacy project page contract failed before upgrade");
  assert(beforeUpgrade.devices.length === 1 && beforeUpgrade.tasks.length === 1 &&
    beforeUpgrade.runs.length === 1 && beforeUpgrade.assets.length === 1, "legacy snapshot is incomplete");

  const migration = await migrateDatabase({ connectionString: postgis.url, logger: silentLogger, through: legacyBoundary });
  assert(migration.total === 51 && migration.applied.length === 51 && migration.applied[0].adopted,
    'legacy TS migrator did not establish the frozen 51-entry ledger');
  const ledgerQuery = 'select name,checksum,adopted,execution_ms,applied_at from schema_migrations order by name';
  const oldLedger = (await client.query(ledgerQuery)).rows;
  assert((await client.query("select to_regclass('public.sessions') as sessions")).rows[0].sessions === null, 'legacy database already has Go sessions');
  const goApplied = await migrateWithGo(postgis.url, 'go-upgrade');
  const newLedger = (await client.query(ledgerQuery)).rows;
  assert(goApplied === migrationCount - 51 && newLedger.length === migrationCount, 'Go did not apply exactly the pending migrations');
  assert(JSON.stringify(newLedger.slice(0, 51)) === JSON.stringify(oldLedger), 'Go changed historical ledger fields');
  assert((await client.query("select to_regclass('public.sessions') as sessions")).rows[0].sessions === 'sessions', 'Go session migration missing');
  assert(await migrateWithGo(postgis.url, 'go-repeat') === 0, 'repeated Go migration reapplied entries');
  assert(JSON.stringify((await client.query(ledgerQuery)).rows) === JSON.stringify(newLedger), 'repeated Go migration changed ledger');
  const afterUpgrade = await readLegacyPageContract(client, scope);
  assert(JSON.stringify(afterUpgrade) === JSON.stringify(beforeUpgrade), "legacy page data changed during upgrade");

  const newAssetId = await writeNewEvidenceAndFutureEvent(client, scope);
  const goService = release ? await verifyGoCutover({ output, databaseURL: postgis.url, client, scope, password, secret }) : null;
  const legacyServices = release ? await verifyLegacyServices({ release, output, databaseURL: postgis.url, scope, password, secret, goProjectId: goService.createdProjectId }) : null;
  if (passwordHash) assert((await client.query('select password from users where id=$1', [scope.userId])).rows[0].password === passwordHash, 'upgrade or old application changed the password');
  assert((await client.query("select to_regclass('public.sessions') as sessions")).rows[0].sessions === 'sessions', 'old application removed additive sessions table');
  await simulateRollbackWorkerClaim(client);
  const afterRollback = await readLegacyPageContract(client, scope);
  assert(afterRollback.project.name === beforeUpgrade.project.name, "rollback project page became unavailable");
  assert(afterRollback.devices.some((row) => row.id === scope.deviceId), "rollback device page lost legacy data");
  assert(afterRollback.tasks.some((row) => row.id === scope.taskId), "rollback task page lost legacy data");
  assert(afterRollback.runs.some((row) => row.id === scope.runId), "rollback run page lost legacy data");
  assert(afterRollback.assets.some((row) => row.id === scope.legacyAssetId), "rollback asset page lost legacy data");

  const evidence = (await client.query(
    `select asset.id,asset.status,asset.legal_hold,count(distinct link.id)::integer as links,
            count(distinct hold.id)::integer as holds
       from assets asset
       left join evidence_links link on link.asset_id=asset.id and link.project_id=asset.project_id
       left join retention_holds hold on hold.asset_id=asset.id and hold.project_id=asset.project_id and hold.status='active'
      where asset.project_id=$1 and asset.id=$2 group by asset.id`, [scope.projectId, newAssetId]
  )).rows[0];
  assert(evidence?.status === "available" && evidence.legal_hold === true && evidence.links === 1 && evidence.holds === 1,
    "new evidence was deleted or detached during rollback");
  const futureEvent = (await client.query(
    `select status,attempts,locked_by,(select count(*)::integer from outbox_consumptions where event_id=outbox.event_id) as consumptions
       from outbox_events outbox where event_id='future-evidence-event'`
  )).rows[0];
  assert(futureEvent?.status === "pending" && futureEvent.attempts === 0 && futureEvent.locked_by === null &&
    futureEvent.consumptions === 0, "rollback worker mutated the unknown event");

  const result = { schemaVersion: 2, generatedAt: new Date().toISOString(), databaseImage,
    scope: legacyServices ? 'real TS-to-Go database upgrade followed by old Next production server and worker against the upgraded database' : 'real TS-to-Go database upgrade and legacy SQL compatibility; old Web/worker processes are not started',
    migration: { total: migrationCount, legacyBoundary, legacyApplied: migration.applied.length, goApplied, repeatedGoApplied: 0, historicalLedgerUnchanged: true, baselineAdopted: true },
    applicationRollbackVerified: Boolean(goService && legacyServices),
    goService,
    legacyServices,
    legacyPageContract: { beforeUpgrade: true, afterUpgrade: true, afterLegacyQueries: true,
      devices: afterRollback.devices.length, tasks: afterRollback.tasks.length,
      runs: afterRollback.runs.length, assetsVisibleToLegacyQuery: afterRollback.assets.length },
    newEvidence: { assetId: newAssetId, status: evidence.status, legalHold: evidence.legal_hold,
      publishedLinks: evidence.links, activeHolds: evidence.holds },
    unknownEvent: futureEvent, passed: true };
  await writeFile(resolve(output, 'result.json'), JSON.stringify(result, null, 2));
  process.stdout.write(`${JSON.stringify(result, null, 2)}\nEvidence: ${output}\n`);
} finally {
  await client.end().catch(() => {});
  postgis.cleanup();
}
