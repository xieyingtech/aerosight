package migrations

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/testdb"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestAIModelCatalogUpgrade(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	source, _ := fs.Sub(Files, "sql")
	list, err := Read(source)
	if err != nil {
		t.Fatal(err)
	}
	previous := fstest.MapFS{}
	for _, migration := range list {
		if migration.Name < "0078_ai_provider_models.sql" {
			previous[migration.Name] = &fstest.MapFile{Data: []byte(migration.SQL)}
		}
	}
	if _, err := Run(ctx, db, previous); err != nil {
		t.Fatal(err)
	}
	if err := database.Bootstrap(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ai_providers(name,provider_type,model_id,base_url,credential_envelope_json,enabled,is_default,realtime_protocol,realtime_model_id,created_by_user_id,updated_by_user_id)
 SELECT 'legacy','openai','text-model','https://legacy.example/v1','{}',true,true,'stepfun','voice-model',id,id FROM users WHERE email='admin@example.com'`); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, db, source); err != nil {
		t.Fatal(err)
	}
	var count int
	var text, voice, protocol string
	var textDefault, voiceDefault bool
	if err := db.QueryRow(`SELECT jsonb_array_length(models_json),models_json->0->>'id',models_json->1->>'id',models_json->1->>'protocol',is_default,is_realtime_default FROM ai_providers WHERE name='legacy'`).Scan(&count, &text, &voice, &protocol, &textDefault, &voiceDefault); err != nil {
		t.Fatal(err)
	}
	if count != 2 || text != "text-model" || voice != "voice-model" || protocol != "stepfun-realtime" || !textDefault || !voiceDefault {
		t.Fatalf("migration lost configuration: %d %s %s %s %v %v", count, text, voice, protocol, textDefault, voiceDefault)
	}
}
