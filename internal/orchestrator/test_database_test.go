package orchestrator

import (
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
)

func testDatabaseDSN(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name())
	return "file:" + name + "_" + model.NewID("testdb") + "?mode=memory&cache=shared"
}
