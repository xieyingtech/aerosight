import { randomBytes, randomUUID } from "node:crypto";
import { spawnSync } from "node:child_process";
import { cpus } from "node:os";

import pg from "pg";

import { migrateDatabase } from "./db-migrate.mjs";
import { createProjectMapModel } from "../lib/project-map-model.ts";
import { readProjectSituationSnapshot } from "../lib/project-snapshot-core.ts";
import {
  mapSuspectedConstructionDetections,
  suspectedConstructionTemplate
} from "../lib/suspected-construction-template.ts";
import { buildTimelineModel } from "../lib/timeline-model.ts";

const { Client, Pool } = pg;
const databaseImage = process.env.BENCHMARK_POSTGIS_IMAGE ?? "postgis/postgis:17-3.5";
const parameters = Object.freeze({
  devices: positiveInteger("BENCHMARK_DEVICES", 12),
  telemetryTicks: positiveInteger("BENCHMARK_TELEMETRY_TICKS", 30),
  telemetryDuplicateDeliveries: positiveInteger("BENCHMARK_TELEMETRY_DUPLICATES", 1),
  detections: positiveInteger("BENCHMARK_DETECTIONS", 30),
  detectionDuplicateDeliveries: positiveInteger("BENCHMARK_DETECTION_DUPLICATES", 1)
});
const thresholds = Object.freeze({ telemetryP95Milliseconds: 2_000, detectionP95Milliseconds: 5_000 });
const silentLogger = { info() {} };

function positiveInteger(name, fallback) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isSafeInteger(value) || value <= 0) throw new Error(`${name} must be a positive integer`);
  return value;
}

function docker(...args) {
  const result = spawnSync("docker", args, { encoding: "utf8" });
  if (result.status !== 0) throw new Error(result.stderr.trim() || `docker ${args[0]} failed`);
  return result.stdout.trim();
}

async function startPostgis() {
  const name = `aerosight-benchmark-${randomBytes(5).toString("hex")}`;
  docker("run", "--detach", "--rm", "--name", name, "--env", "POSTGRES_PASSWORD=aerosight-benchmark",
    "--publish", "127.0.0.1::5432", databaseImage);
  try {
    const port = docker("port", name, "5432/tcp").match(/:(\d+)$/)?.[1];
    if (!port) throw new Error("Could not determine benchmark PostgreSQL port");
    const url = `postgresql://postgres:aerosight-benchmark@127.0.0.1:${port}/postgres`;
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
    throw lastError ?? new Error("Timed out waiting for benchmark PostGIS");
  } catch (error) {
    docker("stop", name);
    throw error;
  }
}

function percentile(values, fraction) {
  if (!values.length) throw new Error("Cannot calculate a percentile without samples");
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.ceil(sorted.length * fraction) - 1];
}

async function seed(client) {
  const user = (await client.query(
    "insert into users(name,email) values('benchmark-owner','benchmark@example.test') returning id"
  )).rows[0];
  const team = (await client.query("insert into teams(name) values('benchmark-team') returning id")).rows[0];
  await client.query("insert into team_members(team_id,user_id,role) values($1,$2,'owner')", [team.id, user.id]);
  const project = (await client.query(
    "insert into projects(team_id,name,created_by_user_id) values($1,'benchmark-project',$2) returning id",
    [team.id, user.id]
  )).rows[0];
  await client.query(
    `insert into project_feature_flags(project_id,operations_overview_enabled,external_algorithms_enabled)
     values($1,true,true)`, [project.id]
  );
  const adapter = (await client.query(
    `insert into device_adapters(project_id,team_id,name,adapter_type,status)
     values($1,$2,'benchmark-simulator','simulator','connected') returning id`, [project.id, team.id]
  )).rows[0];
  const devices = (await client.query(
    `insert into devices(project_id,name,type,status,adapter_id,last_seen_at)
     select $1,'benchmark-drone-' || value,'drone','online',$2,now()
       from generate_series(1,$3::integer) value returning id`, [project.id, adapter.id, parameters.devices]
  )).rows;
  const asset = (await client.query(
    `insert into assets(project_id,team_id,kind,mime_type,storage_key,logical_key,status,captured_at)
     values($1,$2,'image','image/jpeg','benchmark/input.jpg','benchmark/input.jpg','available',now()) returning id`,
    [project.id, team.id]
  )).rows[0];
  const provider = (await client.query(
    `insert into algorithm_providers(project_id,team_id,name,provider_type,base_url,status,created_by_user_id)
     values($1,$2,'benchmark-provider','http-json','https://algorithm.example.test','active',$3) returning id`,
    [project.id, team.id, user.id]
  )).rows[0];
  const definition = (await client.query(
    `insert into algorithm_definitions(project_id,team_id,provider_id,name,capability_code,created_by_user_id)
     values($1,$2,$3,'benchmark-detection','perception.suspected-construction',$4) returning id`,
    [project.id, team.id, provider.id, user.id]
  )).rows[0];
  const algorithmVersion = (await client.query(
    `insert into algorithm_definition_versions(
       project_id,team_id,algorithm_definition_id,version,status,execution_mode,model_or_process,
       output_mapping_json,label_mapping_json,publish_threshold,created_by_user_id,published_by_user_id,published_at
     ) values($1,$2,$3,1,'draft','callback','benchmark-v1',$4,$5,0.65,$6,$6,now()) returning id`,
    [project.id, team.id, definition.id, suspectedConstructionTemplate.outputMapping,
      suspectedConstructionTemplate.labelMapping, user.id]
  )).rows[0];
  await client.query("update algorithm_definition_versions set status='published' where id=$1", [algorithmVersion.id]);
  await client.query("update algorithm_definitions set current_published_version_id=$2 where id=$1", [definition.id, algorithmVersion.id]);
  const rule = (await client.query(
    `insert into event_rules(project_id,team_id,name,status,created_by_user_id)
     values($1,$2,'benchmark-rule','active',$3) returning id`, [project.id, team.id, user.id]
  )).rows[0];
  const ruleVersion = (await client.query(
    `insert into event_rule_versions(
       project_id,team_id,event_rule_id,version,status,label,minimum_confidence,severity,published_by_user_id,published_at
     ) values($1,$2,$3,1,'draft','suspected-construction',0.65,'high',$4,now()) returning id`,
    [project.id, team.id, rule.id, user.id]
  )).rows[0];
  await client.query("update event_rule_versions set status='published' where id=$1", [ruleVersion.id]);
  await client.query("update event_rules set current_published_version_id=$2 where id=$1", [rule.id, ruleVersion.id]);
  return { userId: user.id, teamId: team.id, projectId: project.id, adapterId: adapter.id,
    deviceIds: devices.map((row) => row.id), assetId: asset.id, providerId: provider.id,
    algorithmVersionId: algorithmVersion.id, ruleVersionId: ruleVersion.id };
}

async function visibleSnapshot(pool, scope) {
  const snapshot = await readProjectSituationSnapshot(scope.userId, scope.projectId, () => pool.connect());
  if (!snapshot) throw new Error("Benchmark project snapshot was not readable");
  return { snapshot, map: createProjectMapModel(snapshot), timeline: buildTimelineModel(snapshot) };
}

async function benchmarkTelemetry(client, pool, scope) {
  const latencies = [];
  const capturedBase = Date.now() - parameters.telemetryTicks * 1_000;
  for (let tick = 0; tick < parameters.telemetryTicks; tick += 1) {
    const started = performance.now();
    const capturedAt = new Date(capturedBase + tick * 1_000);
    await client.query("begin");
    try {
      for (let index = 0; index < scope.deviceIds.length; index += 1) {
        const deviceId = scope.deviceIds[index];
        const eventId = `telemetry-${tick}-${deviceId}`;
        const longitude = 120.0 + index * 0.0001 + tick * 0.000001;
        const latitude = 30.0 + index * 0.0001 + tick * 0.000001;
        for (let delivery = 0; delivery <= parameters.telemetryDuplicateDeliveries; delivery += 1) {
          await client.query(
            `insert into device_telemetry(
               project_id,team_id,adapter_id,device_id,event_id,telemetry_type,sequence_number,captured_at,payload_json
             ) values($1,$2,$3,$4,$5,'pose',$6,$7,$8) on conflict do nothing`,
            [scope.projectId, scope.teamId, scope.adapterId, deviceId, eventId, tick, capturedAt,
              { longitude, latitude, altitudeMeters: 50 + index }]
          );
          const observation = await client.query(
            `insert into observations(
               project_id,team_id,adapter_id,device_id,observation_type,source_event_id,captured_at,received_at,
               standard_geometry,properties_json,quality_json
             ) values($1,$2,$3,$4,'pose',$5,$6,now(),ST_SetSRID(ST_MakePoint($7,$8,$9),4326),$10,'{"source":"benchmark"}')
             on conflict(adapter_id,source_event_id) do nothing returning id`,
            [scope.projectId, scope.teamId, scope.adapterId, deviceId, eventId, capturedAt,
              longitude, latitude, 50 + index, { sequenceNumber: tick }]
          );
          if (observation.rowCount === 1) {
            await client.query(
              `insert into poses(
                 observation_id,project_id,device_id,captured_at,standard_position,horizontal_accuracy_m,
                 vertical_accuracy_m,vertical_datum,transform_version,spatial_quality
               ) values($1,$2,$3,$4,ST_SetSRID(ST_MakePoint($5,$6,$7),4326),0.5,1,'WGS84 ellipsoid','benchmark/v1','usable')`,
              [observation.rows[0].id, scope.projectId, deviceId, capturedAt, longitude, latitude, 50 + index]
            );
          }
        }
        await client.query("update devices set status='online',last_seen_at=$2 where id=$1", [deviceId, capturedAt]);
      }
      await client.query("commit");
    } catch (error) {
      await client.query("rollback");
      throw error;
    }
    const view = await visibleSnapshot(pool, scope);
    if (view.map.features.filter((item) => item.properties.layerKind.startsWith("device-")).length !== parameters.devices) {
      throw new Error(`Telemetry tick ${tick} was not visible for every device on the map`);
    }
    if (!view.timeline.lanes.find((lane) => lane.key === "devices")?.items.length) {
      throw new Error(`Telemetry tick ${tick} was not visible on the timeline`);
    }
    latencies.push(performance.now() - started);
  }
  const counts = (await client.query(
    `select (select count(*)::integer from device_telemetry) as telemetry,
            (select count(*)::integer from observations) as observations,
            (select count(*)::integer from poses) as poses`
  )).rows[0];
  const expected = parameters.devices * parameters.telemetryTicks;
  if (counts.telemetry !== expected || counts.observations !== expected || counts.poses !== expected) {
    throw new Error(`Telemetry duplicate side effect: expected ${expected}, got ${JSON.stringify(counts)}`);
  }
  return { samples: latencies.length, p50Milliseconds: percentile(latencies, 0.5),
    p95Milliseconds: percentile(latencies, 0.95), maxMilliseconds: Math.max(...latencies),
    uniqueEvents: expected, duplicateDeliveries: expected * parameters.telemetryDuplicateDeliveries,
    sideEffects: counts };
}

async function benchmarkDetections(client, pool, scope) {
  const latencies = [];
  for (let index = 0; index < parameters.detections; index += 1) {
    const runId = randomUUID();
    const eventId = randomUUID();
    const callbackId = `benchmark-callback-${index}`;
    await client.query(
      `insert into algorithm_runs(
         id,project_id,team_id,algorithm_definition_version_id,input_asset_id,idempotency_key,status
       ) values($1,$2,$3,$4,$5,$6,'succeeded')`,
      [runId, scope.projectId, scope.teamId, scope.algorithmVersionId, scope.assetId, `benchmark-run-${index}`]
    );
    const started = performance.now();
    const canonical = mapSuspectedConstructionDetections({
      response: { results: [{ id: `detection-${index}`, class: "suspected_construction", score: 0.91,
        geometry: { type: "bbox", x: 10, y: 12, width: 30, height: 24 } }] },
      mapping: suspectedConstructionTemplate.outputMapping,
      labelMapping: suspectedConstructionTemplate.labelMapping,
      inputAsset: { assetId: scope.assetId, version: 1, checksumSha256: "a".repeat(64), mimeType: "image/jpeg" }
    })[0];
    await client.query("begin");
    try {
      for (let delivery = 0; delivery <= parameters.detectionDuplicateDeliveries; delivery += 1) {
        const receipt = await client.query(
          `insert into algorithm_callback_receipts(
             project_id,team_id,algorithm_run_id,provider_id,callback_id,external_job_id,payload_hash,disposition
           ) values($1,$2,$3,$4,$5,$6,$7,'applied')
           on conflict(provider_id,callback_id) do nothing returning id`,
          [scope.projectId, scope.teamId, runId, scope.providerId, callbackId, `job-${index}`, "b".repeat(64)]
        );
        if (receipt.rowCount === 0) continue;
        const capturedAt = new Date();
        const longitude = 120.2 + index * 0.0001;
        const latitude = 30.2 + index * 0.0001;
        const detection = (await client.query(
          `insert into detections(
             project_id,team_id,algorithm_run_id,input_asset_id,detection_key,label,confidence,pixel_geometry_json,
             geographic_geometry,location_quality,projection_method,horizontal_error_meters,transform_version,captured_at
           ) values($1,$2,$3,$4,$5,$6,$7,$8,
             ST_MakeEnvelope($9::double precision,$10::double precision,
               $9::double precision+0.00005,$10::double precision+0.00005,4326),
             'estimated','benchmark-projection',2,'benchmark/v1',$11)
           returning id`,
          [scope.projectId, scope.teamId, runId, scope.assetId, canonical.detectionKey, canonical.label,
            canonical.confidence, canonical.pixelGeometry, longitude, latitude, capturedAt]
        )).rows[0];
        const group = (await client.query(
          `insert into detection_groups(
             project_id,team_id,label,geographic_geometry,location_quality,first_detected_at,last_detected_at
           ) values($1,$2,$3,ST_MakeEnvelope($4::double precision,$5::double precision,
             $4::double precision+0.00005,$5::double precision+0.00005,4326),'estimated',$6,$6) returning id`,
          [scope.projectId, scope.teamId, canonical.label, longitude, latitude, capturedAt]
        )).rows[0];
        await client.query(
          `insert into detection_group_members(project_id,team_id,detection_group_id,detection_id)
           values($1,$2,$3,$4)`, [scope.projectId, scope.teamId, group.id, detection.id]
        );
        await client.query(
          `insert into perception_events(
             id,project_id,team_id,event_rule_version_id,detection_group_id,deduplication_key,severity,
             first_detected_at,last_detected_at
           ) values($1,$2,$3,$4,$5,$6,'high',$7,$7)`,
          [eventId, scope.projectId, scope.teamId, scope.ruleVersionId, group.id,
            `benchmark-rule:${scope.ruleVersionId}:group:${group.id}`, capturedAt]
        );
      }
      await client.query("commit");
    } catch (error) {
      await client.query("rollback");
      throw error;
    }
    const view = await visibleSnapshot(pool, scope);
    if (!view.snapshot.openAlerts.some((alert) => alert.id === eventId)) {
      throw new Error(`Canonical detection ${index} did not become a visible alert`);
    }
    latencies.push(performance.now() - started);
  }
  const counts = (await client.query(
    `select (select count(*)::integer from algorithm_callback_receipts) as callbacks,
            (select count(*)::integer from detections) as detections,
            (select count(*)::integer from detection_groups) as groups,
            (select count(*)::integer from perception_events) as alerts`
  )).rows[0];
  for (const value of Object.values(counts)) {
    if (value !== parameters.detections) throw new Error(`Detection duplicate side effect: ${JSON.stringify(counts)}`);
  }
  return { samples: latencies.length, p50Milliseconds: percentile(latencies, 0.5),
    p95Milliseconds: percentile(latencies, 0.95), maxMilliseconds: Math.max(...latencies),
    uniqueEvents: parameters.detections,
    duplicateDeliveries: parameters.detections * parameters.detectionDuplicateDeliveries, sideEffects: counts };
}

const postgis = await startPostgis();
let pool;
try {
  await migrateDatabase({ connectionString: postgis.url, logger: silentLogger });
  pool = new Pool({ connectionString: postgis.url, max: 4 });
  const client = await pool.connect();
  try {
    const scope = await seed(client);
    const telemetry = await benchmarkTelemetry(client, pool, scope);
    const detections = await benchmarkDetections(client, pool, scope);
    const result = {
      schemaVersion: 1,
      generatedAt: new Date().toISOString(),
      environment: { node: process.version, platform: process.platform, architecture: process.arch,
        logicalCpus: cpus().length, databaseImage },
      scope: "PostGIS persistence, idempotency constraints, repeatable-read project snapshot, map and timeline projections",
      parameters,
      thresholds,
      results: { telemetry, detections },
      passed: telemetry.p95Milliseconds <= thresholds.telemetryP95Milliseconds &&
        detections.p95Milliseconds <= thresholds.detectionP95Milliseconds
    };
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
    if (!result.passed) process.exitCode = 1;
  } finally {
    client.release();
  }
} finally {
  await pool?.end().catch(() => {});
  postgis.cleanup();
}
