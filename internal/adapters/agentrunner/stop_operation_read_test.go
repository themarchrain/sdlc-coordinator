package agentrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/workspaceproto"
)

func TestDeliveryOperationReadUsesPinnedRunnerAndTenant(t *testing.T) {
	for _, tenantID := range []uint{7, 8} {
		t.Run(fmt.Sprint(tenantID), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/internal/operations/push-run" || r.Header.Get("Authorization") != "Bearer deployment-token" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(workspaceproto.OperationRecord{OperationID: "push-run", TenantID: tenantID})
			}))
			defer server.Close()
			remote := NewRoutedRemoteWorkspace("http://unused.invalid", "deployment-token", 7, server.Client(), func(context.Context, string) (string, error) { return server.URL, nil })
			record, err := remote.ReadDeliveryOperation(context.Background(), "run", "push-run")
			if calls != 1 || (tenantID == 7 && (err != nil || record == nil)) || (tenantID != 7 && (err == nil || record != nil)) {
				t.Fatalf("tenant boundary: record=%+v err=%v calls=%d", record, err, calls)
			}
		})
	}
}
