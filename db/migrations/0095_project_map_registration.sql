CREATE TABLE project_map_regions (
 id bigserial PRIMARY KEY,
 project_id integer NOT NULL,
 team_id integer NOT NULL,
 registration_key text NOT NULL,
 name text NOT NULL,
 geometry geometry(MultiPolygon,4326) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (project_id,team_id) REFERENCES projects(id,team_id) ON DELETE CASCADE,
 UNIQUE (project_id,registration_key),
 CHECK (ST_IsValid(geometry) AND NOT ST_IsEmpty(geometry))
);
--> statement-breakpoint
ALTER TABLE devices ADD COLUMN registration_key text;
--> statement-breakpoint
CREATE UNIQUE INDEX devices_project_registration_key ON devices(project_id,registration_key) WHERE registration_key IS NOT NULL;
--> statement-breakpoint
INSERT INTO device_types(type_key,version,display_name,category,driver_definition_id,icon)
SELECT entry.key,1,entry.name,entry.category,driver.id,entry.icon
FROM (VALUES
 ('registered.fixed-camera','固定监控','camera','cctv'),
 ('registered.patrol-vehicle','巡检车','ground_robot','car'),
 ('registered.environment-sensor','环境传感器','sensor','radio-tower')
) AS entry(key,name,category,icon)
JOIN driver_definitions driver ON driver.driver_key='legacy.static' AND driver.version='1.0.0'
ON CONFLICT(type_key,version) DO NOTHING;
