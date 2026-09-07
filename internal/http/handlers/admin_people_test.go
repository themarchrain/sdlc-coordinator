package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/store"

	"github.com/gin-gonic/gin"
)

func TestAdminPeopleHandlerCreatesListsAndSoftDeletesPeople(t *testing.T) {
	router := newAdminPeopleRouter(t, &fakePeopleLookup{})
	created := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people", map[string]any{
		"display_name": "Alice", "email": "user@example.com", "enabled": true,
		"identities": []map[string]any{{"provider": "gitlab", "base_url": "https://gitlab.example.com", "external_user_id": "7", "username": "user"}},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("unexpected create response %d: %s", created.Code, created.Body.String())
	}
	var person struct {
		PersonID string `json:"person_id"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &person); err != nil || person.PersonID == "" || person.Email != "user@example.com" {
		t.Fatalf("decode created person: person=%#v error=%v", person, err)
	}

	listed := performAdminPeopleJSON(t, router, http.MethodGet, "/api/admin/people?q=user&page=1&page_size=20", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), person.PersonID) {
		t.Fatalf("unexpected list response %d: %s", listed.Code, listed.Body.String())
	}

	deleted := performAdminPeopleJSON(t, router, http.MethodDelete, "/api/admin/people/"+person.PersonID, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("unexpected delete response %d: %s", deleted.Code, deleted.Body.String())
	}
	notFound := performAdminPeopleJSON(t, router, http.MethodGet, "/api/admin/people/"+person.PersonID, nil)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected soft-deleted person to be hidden, got %d: %s", notFound.Code, notFound.Body.String())
	}
}

func TestAdminPeopleHandlerReturnsLookupCandidatesWithoutToken(t *testing.T) {
	lookup := &fakePeopleLookup{result: people.LookupResult{Candidates: []people.LookupCandidate{{ExternalUserID: "7", Username: "user", DisplayName: "用户"}}}}
	router := newAdminPeopleRouter(t, lookup)

	response := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people/codehost-identities/lookup", map[string]any{
		"provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "temporary-token", "email": "user@example.com",
	})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "temporary-token") || !strings.Contains(response.Body.String(), `"external_user_id":"7"`) {
		t.Fatalf("unexpected lookup response %d: %s", response.Code, response.Body.String())
	}
	if lookup.request.Token != "temporary-token" {
		t.Fatalf("lookup did not receive token")
	}
}

func TestAdminPeopleHandlerMapsLookupAuthenticationFailure(t *testing.T) {
	router := newAdminPeopleRouter(t, &fakePeopleLookup{err: people.ErrGitLabAuthFailed})

	response := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people/codehost-identities/lookup", map[string]any{
		"provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "temporary-token", "username": "user",
	})
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "gitlab_auth_failed") || strings.Contains(response.Body.String(), "temporary-token") {
		t.Fatalf("unexpected authentication error %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminPeopleHandlerPreviewsBatchLookupWithoutEchoingToken(t *testing.T) {
	lookup := &fakePeopleLookup{result: people.LookupResult{Candidates: []people.LookupCandidate{{ExternalUserID: "7", Username: "draft", DisplayName: "待补全"}}}}
	router := newAdminPeopleRouter(t, lookup)
	created := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people", map[string]any{
		"display_name": "待补全", "email": "draft@example.com", "enabled": false,
		"identities": []map[string]any{{"provider": "gitlab", "base_url": "https://gitlab.example.com", "external_user_id": "", "username": ""}},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create draft person: %d %s", created.Code, created.Body.String())
	}

	response := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people/codehost-identities/lookup-batch", map[string]any{
		"provider": "gitlab", "base_url": "https://gitlab.example.com", "token": "temporary-token",
	})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "temporary-token") || !strings.Contains(response.Body.String(), `"person_id"`) || !strings.Contains(response.Body.String(), `"external_user_id":"7"`) {
		t.Fatalf("unexpected batch response %d: %s", response.Code, response.Body.String())
	}
	if lookup.request.Email != "draft@example.com" || lookup.request.Username != "" {
		t.Fatalf("batch lookup did not derive username from email: %#v", lookup.request)
	}
}

func TestAdminPeopleHandlerRejectsIncompleteEnabledPerson(t *testing.T) {
	router := newAdminPeopleRouter(t, &fakePeopleLookup{})
	response := performAdminPeopleJSON(t, router, http.MethodPost, "/api/admin/people", map[string]any{
		"display_name": "待补全", "email": "draft@example.com", "enabled": true,
		"identities": []map[string]any{{"provider": "gitlab", "base_url": "https://gitlab.example.com", "external_user_id": "", "username": ""}},
	})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "incomplete_codehost_identity") {
		t.Fatalf("unexpected incomplete identity response %d: %s", response.Code, response.Body.String())
	}
}

func newAdminPeopleRouter(t *testing.T, lookup people.IdentityLookup) *gin.Engine {
	t.Helper()
	db, err := store.Open("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repository := people.NewRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate people: %v", err)
	}
	router := gin.New()
	NewAdminPeopleHandler(people.NewService(repository), lookup).RegisterRoutes(router.Group("/api/admin"))
	return router
}

func performAdminPeopleJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakePeopleLookup struct {
	result  people.LookupResult
	err     error
	request people.LookupRequest
}

func (lookup *fakePeopleLookup) Lookup(_ context.Context, request people.LookupRequest) (people.LookupResult, error) {
	lookup.request = request
	return lookup.result, lookup.err
}
