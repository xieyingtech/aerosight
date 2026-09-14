package httpapi

import (
	"aerosight/server/internal/taskdefinition"
	"os/exec"
	"testing"
)

// Integration acceptance uses the exact template exported by the product,
// including default schemas, dependsOn and report scope. Only fixture resource
// IDs are substituted by callers; no parallel copy of the template is maintained.
func productInspectionTemplate(t *testing.T, mode string) map[string]any {
	t.Helper()
	command := exec.Command("node", "--input-type=module", "-e", `import {inspectionTaskTemplate} from './lib/inspection-task-templates.ts'; process.stdout.write(inspectionTaskTemplate(process.argv[1]).source);`, mode)
	command.Dir = "../../../web"
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("read product template (requires installed web dependencies): %v: %s", err, raw)
	}
	definition, err := taskdefinition.Parse("yaml", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}
