package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestPermissionsMatchFrozenLegacyMatrix(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/go-migration/permission-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Role                string
			Grants, Permissions []string
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 45 {
		t.Fatalf("incomplete baseline: %d cases", len(fixture.Cases))
	}
	for _, row := range fixture.Cases {
		actual := []string{}
		for permission, allowed := range effectivePermissions(row.Role, row.Grants) {
			if allowed {
				actual = append(actual, permission)
			}
		}
		sort.Strings(actual)
		if !reflect.DeepEqual(actual, row.Permissions) {
			t.Errorf("role=%s grants=%v: got %v want %v", row.Role, row.Grants, actual, row.Permissions)
		}
	}
}
