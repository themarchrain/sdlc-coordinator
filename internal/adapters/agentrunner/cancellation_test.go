package agentrunner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdlc-coordinator/internal/runner"
)

func TestCancellationProtocolRequiresInternalAuthentication(t *testing.T) {
	for _, token := range []string{"", "wrong", "deployment-token"} {
		t.Run(token, func(t *testing.T) {
			service, err := runner.NewService(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(runner.NewHTTPHandler(service, runner.WithWorkspaceOperations(nil, "deployment-token")))
			defer server.Close()
			client := NewWorkspaceClient(server.URL, token, server.Client())
			ack, err := client.CancelDurable(context.Background(), "run", runner.CancellationRequest{StopID: "stop-one", TaskID: "task"})
			if token == "deployment-token" {
				if err != nil || ack.StopID != "stop-one" || ack.ExecutionID != "run" || !ack.FutureCreateBlocked || ack.ExitConfirmed {
					t.Fatalf("bad proof: %+v %v", ack, err)
				}
			} else {
				var httpErr *runner.HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
					t.Fatalf("unauthorized cancellation accepted: %v", err)
				}
			}
		})
	}
}
