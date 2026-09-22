ALTER TABLE ai_providers
 ADD COLUMN models_json jsonb NOT NULL DEFAULT '[]'::jsonb,
 ADD COLUMN is_realtime_default boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT ai_providers_models_array CHECK (jsonb_typeof(models_json) = 'array'),
 ADD CONSTRAINT ai_providers_realtime_default_enabled CHECK (NOT is_realtime_default OR (enabled AND realtime_protocol <> 'disabled'));
--> statement-breakpoint
UPDATE ai_providers SET models_json =
 jsonb_build_array(jsonb_build_object('id', model_id, 'protocol', 'responses', 'capabilities', jsonb_build_array('text'), 'enabled', true)) ||
 CASE WHEN realtime_protocol <> 'disabled' THEN jsonb_build_array(jsonb_build_object('id', realtime_model_id, 'protocol', 'stepfun-realtime', 'capabilities', jsonb_build_array('realtime', 'audio-input', 'audio-output'), 'enabled', true)) ELSE '[]'::jsonb END,
 is_realtime_default = enabled AND is_default AND realtime_protocol <> 'disabled';
--> statement-breakpoint
CREATE UNIQUE INDEX ai_providers_single_realtime_default_idx ON ai_providers(is_realtime_default) WHERE is_realtime_default;
