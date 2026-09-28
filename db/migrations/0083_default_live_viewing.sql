ALTER TABLE project_feature_flags
  ALTER COLUMN flighthub_action_flags_json SET DEFAULT '{"live.control":true}'::jsonb;
--> statement-breakpoint
UPDATE project_feature_flags
SET flighthub_action_flags_json = '{"live.control":true}'::jsonb || flighthub_action_flags_json
WHERE NOT (flighthub_action_flags_json ? 'live.control');
