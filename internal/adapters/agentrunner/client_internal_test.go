package agentrunner

import "testing"

func TestNewClientUsesFiniteDefaultHTTPTimeout(t *testing.T) {
	client := NewClient("http://agent-runner:8090", nil)
	if client.http == nil || client.http.Timeout <= 0 {
		t.Fatalf("expected finite default HTTP timeout, got %#v", client.http)
	}
}
