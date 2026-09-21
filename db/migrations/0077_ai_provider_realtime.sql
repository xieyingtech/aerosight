ALTER TABLE ai_providers
 ADD COLUMN realtime_protocol text NOT NULL DEFAULT 'disabled',
 ADD COLUMN realtime_model_id text NOT NULL DEFAULT '',
 ADD CONSTRAINT ai_providers_realtime_valid CHECK (
   (realtime_protocol = 'disabled' AND realtime_model_id = '') OR
   (realtime_protocol = 'stepfun' AND length(btrim(realtime_model_id)) BETWEEN 1 AND 255)
 );
