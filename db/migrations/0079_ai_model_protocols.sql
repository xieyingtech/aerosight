UPDATE ai_providers p SET models_json = (
 SELECT coalesce(jsonb_agg(
  CASE WHEN coalesce(m->>'protocol','') IN ('', 'catalog')
   THEN jsonb_set(m, '{protocol}', '"openai-compatible"'::jsonb)
   ELSE m END ORDER BY position
 ), '[]'::jsonb)
 FROM jsonb_array_elements(p.models_json) WITH ORDINALITY AS entry(m, position)
);
