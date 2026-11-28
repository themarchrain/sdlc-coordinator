package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestServiceUpdateRetainsReplacesAndExplicitlyClearsSecrets(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	initial.AgentRunner.APIKey = "old-runner-key"
	initial.AgentRunner.CustomHeadersJSON = `{"X-Old":"secret"}`
	saved, err := repository.Save(context.Background(), StatusReady, SourceUI, initial)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: saved})
	input := EditableFromRuntime(initial)
	input.AgentRunner.Model = "updated-model"

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{
		Config: input, ExpectedRevision: uint64Pointer(1),
		Secrets: SecretChanges{
			AgentRunnerAPIKey:                 stringPointer("new-runner-key"),
			ClearAgentRunnerCustomHeadersJSON: true,
		},
	})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update runtime configuration: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}

	latest, err := repository.Latest(context.Background())
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if latest.Config.AgentRunner.APIKey != "new-runner-key" || latest.Config.AgentRunner.CustomHeadersJSON != "" {
		t.Fatalf("agent runner secret changes were not applied: %#v", latest.Config.AgentRunner)
	}
	if latest.Config.AgentRunner.Model != "updated-model" || latest.Status != StatusRestartRequired {
		t.Fatalf("unexpected latest configuration: %#v", latest)
	}
	if result.Mode != ModeRestartRequired || result.Revision != latest.Revision || result.Configuration.Revision != latest.Revision {
		t.Fatalf("unexpected update result: %#v", result)
	}
}

func TestServiceUpdateKeepsCallbackWhenSubmissionOmitsIt(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	saved, err := repository.Save(context.Background(), StatusReady, SourceUI, initial)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: saved})
	input := EditableFromRuntime(initial)
	input.Scheduler.CallbackBaseURL = ""

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update runtime configuration: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}
	if result.Configuration.Config.Scheduler.CallbackBaseURL != "https://coordinator.example.com" {
		t.Fatalf("omitted scheduler fields were not retained: %#v", result.Configuration.Config.Scheduler)
	}
}

func TestServiceUpdateDoesNotPersistWorkspacePaths(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	saved, err := repository.Save(context.Background(), StatusReady, SourceUI, initial)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: saved})
	request := UpdateRequest{Config: EditableFromRuntime(initial), ExpectedRevision: uint64Pointer(1)}
	request.Config.AgentRunner.Model = "updated-model"
	result, validationErrors, err := service.Update(context.Background(), request)
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update configuration: validation=%v error=%v", validationErrors, err)
	}
	latest, err := repository.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var record SnapshotRecord
	if err := repository.db.Where("revision = ?", result.Revision).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.PayloadJSON, "workspace_root") {
		t.Fatalf("deployment-only workspace paths persisted in runtime snapshot: %s", record.PayloadJSON)
	}
	raw, err := json.Marshal(result.Configuration)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "workspace_root") {
		t.Fatalf("configuration response exposes deployment-only workspace paths: %s", raw)
	}
	if latest.Config.AgentRunner.Model != "updated-model" || result.Revision != 2 {
		t.Fatal("configuration update was not persisted")
	}
}

func TestServiceUpdateDropsLegacyTranslatorFields(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("update without translator settings: result=%#v validation=%#v error=%v", result, validationErrors, err)
	}
	raw, err := json.Marshal(result.Configuration.Config)
	if err != nil {
		t.Fatalf("marshal persisted config: %v", err)
	}
	if strings.Contains(string(raw), "translator_url") || strings.Contains(string(raw), "normalize_to_chat") {
		t.Fatalf("legacy translator fields were persisted: %s", raw)
	}
}

func TestServiceViewNeverContainsSecretValues(t *testing.T) {
	service := newReadyConfigurationService(t)

	view := service.Get()

	if !view.Secrets.AgentRunnerAPIKeyConfigured || !view.Secrets.AgentRunnerCustomHeadersConfigured {
		t.Fatalf("configured flags are incomplete: %#v", view.Secrets)
	}
	if view.Config.AgentRunner.ServiceURL == "" || view.Config.AgentRunner.Model == "" {
		t.Fatalf("non-secret agent runner values missing: %#v", view.Config.AgentRunner)
	}
}

func TestServiceRejectsInvalidUpdateWithoutCreatingRevision(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config
	input.Scheduler.CallbackBaseURL = "not-a-url"

	result, validationErrors, err := service.Update(context.Background(), UpdateRequest{Config: input, ExpectedRevision: uint64Pointer(1)})

	if err != nil {
		t.Fatalf("invalid request should return field errors, got %v", err)
	}
	if result != nil || !validationErrors.Has("scheduler.callback_base_url") {
		t.Fatalf("unexpected invalid update result=%#v validation=%#v", result, validationErrors)
	}
	status := service.Status()
	if status.Revision != 1 || status.Mode != ModeReady {
		t.Fatalf("invalid update changed service state: %#v", status)
	}
}

func TestServiceValidationDoesNotPersistConfiguration(t *testing.T) {
	service := newReadyConfigurationService(t)
	input := service.Get().Config
	input.Scheduler.Interval = "60s"

	validationErrors := service.Validate(UpdateRequest{Config: input})

	if len(validationErrors) != 0 {
		t.Fatalf("expected valid configuration, got %#v", validationErrors)
	}
	if status := service.Status(); status.Revision != 1 || status.Mode != ModeReady {
		t.Fatalf("validation persisted changes: %#v", status)
	}
}

func TestServiceRevisionsListsMaskedRestorableHistoryNewestFirst(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	first := validRuntimeConfig()
	first.AgentRunner.Model = "model-one"
	second := validRuntimeConfig()
	second.AgentRunner.Model = "model-two"
	second.AgentRunner.APIKey = "rotated-key"
	if _, err := repository.Save(ctx, StatusReady, SourceEnvImport, first); err != nil {
		t.Fatalf("save first snapshot: %v", err)
	}
	if _, err := repository.Save(ctx, StatusRestartRequired, SourceUI, second); err != nil {
		t.Fatalf("save second snapshot: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady})

	revisions, total, err := service.Revisions(ctx, 1, 20)
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if total != 2 || len(revisions) != 2 {
		t.Fatalf("expected two revisions, got total=%d len=%d", total, len(revisions))
	}
	if revisions[0].Revision != 2 || revisions[1].Revision != 1 {
		t.Fatalf("expected newest-first [2 1], got %#v", revisions)
	}
	if revisions[0].Source != SourceUI || revisions[1].Source != SourceEnvImport {
		t.Fatalf("revision sources were not carried: %#v", revisions)
	}
	if revisions[0].Status != StatusRestartRequired || revisions[1].Status != StatusReady {
		t.Fatalf("revision statuses were not carried: %#v", revisions)
	}
	for _, view := range revisions {
		if !view.Restorable {
			t.Fatalf("valid historical revision marked unrestorable: %#v", view)
		}
		if view.Config == nil || view.Secrets == nil {
			t.Fatalf("decodable revision is missing masked view: %#v", view)
		}
	}
	if !revisions[0].Secrets.AgentRunnerAPIKeyConfigured {
		t.Fatalf("secret configured state missing: %#v", revisions[0].Secrets)
	}
	raw, err := json.Marshal(revisions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "rotated-key") {
		t.Fatalf("revision history leaks secret values: %s", raw)
	}
}

func TestServiceRevisionsMarksSchemaExpiredRowUnrestorable(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	if err := repository.db.Create(&SnapshotRecord{
		Revision:    7,
		Status:      string(StatusReady),
		Source:      string(SourceUI),
		PayloadJSON: "{not-json",
	}).Error; err != nil {
		t.Fatalf("seed schema-expired row: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady})

	revisions, total, err := service.Revisions(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("list revisions with schema-expired row: %v", err)
	}
	if total != 1 || len(revisions) != 1 {
		t.Fatalf("expected the legacy row to be listed, got total=%d len=%d", total, len(revisions))
	}
	view := revisions[0]
	if view.Restorable {
		t.Fatalf("schema-expired row must not be restorable: %#v", view)
	}
	if !strings.Contains(view.UnrestorableReason, "schema 过期") {
		t.Fatalf("unreadable schema-expired reason: %q", view.UnrestorableReason)
	}
	if view.Config != nil || view.Secrets != nil {
		t.Fatalf("undecodable row must not carry a config view: %#v", view)
	}
}

func TestServiceRevisionsMarksConfigFailingCurrentRulesUnrestorable(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	invalid := validRuntimeConfig()
	invalid.AgentRunner.Model = ""
	payload, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.db.Create(&SnapshotRecord{
		Revision:    1,
		Status:      string(StatusReady),
		Source:      string(SourceUI),
		PayloadJSON: string(payload),
	}).Error; err != nil {
		t.Fatalf("seed rule-expired row: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady})

	revisions, _, err := service.Revisions(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("list revisions with rule-expired row: %v", err)
	}
	if len(revisions) != 1 {
		t.Fatalf("expected one revision, got %#v", revisions)
	}
	if revisions[0].Restorable {
		t.Fatalf("config failing current rules must not be restorable: %#v", revisions[0])
	}
	if !strings.Contains(revisions[0].UnrestorableReason, "校验") {
		t.Fatalf("unreadable rule-expired reason: %q", revisions[0].UnrestorableReason)
	}
	if revisions[0].Config == nil {
		t.Fatalf("rule-expired row is still decodable and should show a masked view: %#v", revisions[0])
	}
}

func TestServiceRevisionReturnsSingleMaskedViewAndNotFound(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady})

	view, err := service.Revision(context.Background(), 1)
	if err != nil {
		t.Fatalf("load revision 1: %v", err)
	}
	if view.Revision != 1 || !view.Restorable || view.Config == nil || !view.Secrets.AgentRunnerAPIKeyConfigured {
		t.Fatalf("unexpected revision view: %#v", view)
	}

	_, err = service.Revision(context.Background(), 42)
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected snapshot not found, got %v", err)
	}
}

func TestServiceRevisionsPaginatesNewestFirst(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	for index := 0; index < 3; index++ {
		config := validRuntimeConfig()
		config.AgentRunner.Model = fmt.Sprintf("model-%d", index)
		if _, err := repository.Save(ctx, StatusReady, SourceUI, config); err != nil {
			t.Fatalf("save snapshot %d: %v", index, err)
		}
	}
	service := NewService(repository, Resolution{Mode: ModeReady})

	revisions, total, err := service.Revisions(ctx, 2, 2)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if total != 3 || len(revisions) != 1 || revisions[0].Revision != 1 {
		t.Fatalf("unexpected second page: total=%d revisions=%#v", total, revisions)
	}
}

func TestServiceRestoreAppendsRestoredRevisionFromHistory(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	initial := validRuntimeConfig()
	initial.AgentRunner.APIKey = "old-runner-key"
	initial.AgentRunner.CustomHeadersJSON = `{"X-Old":"v"}`
	initial.AgentRunner.Model = "model-a"
	if _, err := repository.Save(ctx, StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})
	broken := EditableFromRuntime(initial)
	broken.AgentRunner.Model = "model-b"
	newerRevision := uint64(1)
	result, validationErrors, err := service.Update(ctx, UpdateRequest{
		Config: broken, ExpectedRevision: &newerRevision,
		Secrets: SecretChanges{
			AgentRunnerAPIKey:                 stringPointer("new-runner-key"),
			ClearAgentRunnerCustomHeadersJSON: true,
		},
	})
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("prepare broken revision: validation=%#v error=%v", validationErrors, err)
	}
	if result.Revision != 2 {
		t.Fatalf("unexpected broken revision: %d", result.Revision)
	}
	secondRecord, err := repository.GetRecord(ctx, 2)
	if err != nil {
		t.Fatalf("load broken record: %v", err)
	}
	if secondRecord.Source != string(SourceUI) {
		t.Fatalf("manual save must keep the ui source: %s", secondRecord.Source)
	}

	restoreExpected := uint64(2)
	restored, validationErrors, err := service.Restore(ctx, 1, &restoreExpected)
	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("restore revision 1: validation=%#v error=%v", validationErrors, err)
	}
	if restored.Revision != 3 || restored.Mode != ModeRestartRequired {
		t.Fatalf("unexpected restore result: %#v", restored)
	}
	if restored.Source != SourceRestore {
		t.Fatalf("restored revision must carry the restore source: %#v", restored.StatusView)
	}

	latest, err := repository.Latest(ctx)
	if err != nil {
		t.Fatalf("load restored revision: %v", err)
	}
	if latest.Revision != 3 || latest.Status != StatusRestartRequired || latest.Source != SourceRestore {
		t.Fatalf("unexpected restored snapshot: %#v", latest)
	}
	if latest.Config.AgentRunner.Model != "model-a" {
		t.Fatalf("non-secret fields were not restored: %#v", latest.Config.AgentRunner)
	}
	if latest.Config.AgentRunner.APIKey != "old-runner-key" {
		t.Fatalf("historical api key was not restored (方案 A): %#v", latest.Config.AgentRunner)
	}
	if latest.Config.AgentRunner.CustomHeadersJSON != `{"X-Old":"v"}` {
		t.Fatalf("historical headers were not restored: %#v", latest.Config.AgentRunner)
	}
	for revision := uint64(1); revision <= 2; revision++ {
		record, err := repository.GetRecord(ctx, revision)
		if err != nil {
			t.Fatalf("load history revision %d: %v", revision, err)
		}
		if revision == 1 && (record.Source != string(SourceUI) || record.Revision != 1) {
			t.Fatalf("restoring mutated historical revision 1: %#v", record)
		}
	}
	view, err := service.Revision(ctx, 1)
	if err != nil {
		t.Fatalf("reload revision 1 view: %v", err)
	}
	if !view.Restorable {
		t.Fatalf("history view changed after restore: %#v", view)
	}
}

func TestServiceRestoreRejectsStaleExpectedRevisionWithoutWriting(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	initial := validRuntimeConfig()
	if _, err := repository.Save(ctx, StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})
	updated := EditableFromRuntime(initial)
	updated.AgentRunner.Model = "model-b"
	newerRevision := uint64(1)
	if _, validationErrors, err := service.Update(ctx, UpdateRequest{Config: updated, ExpectedRevision: &newerRevision}); err != nil || len(validationErrors) != 0 {
		t.Fatalf("prepare revision 2: validation=%#v error=%v", validationErrors, err)
	}

	stale := uint64(1)
	result, validationErrors, err := service.Restore(ctx, 1, &stale)

	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict, got err=%v result=%#v", err, result)
	}
	if result != nil || validationErrors != nil {
		t.Fatalf("conflict must not carry a result: result=%#v validation=%#v", result, validationErrors)
	}
	latest, err := repository.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 2 || latest.Config.AgentRunner.Model != "model-b" {
		t.Fatalf("stale restore wrote a revision: %#v", latest)
	}
}

func TestServiceRestoreRejectsMissingRevision(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})

	expected := uint64(1)
	result, validationErrors, err := service.Restore(context.Background(), 42, &expected)

	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected snapshot not found, got err=%v result=%#v", err, result)
	}
	if result != nil || validationErrors != nil {
		t.Fatalf("missing revision must not carry a result: result=%#v validation=%#v", result, validationErrors)
	}
}

func TestServiceRestoreRejectsSchemaExpiredRevision(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	initial := validRuntimeConfig()
	if _, err := repository.Save(ctx, StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})
	if err := repository.db.Create(&SnapshotRecord{
		Revision:    2,
		Status:      string(StatusReady),
		Source:      string(SourceUI),
		PayloadJSON: "{not-json",
	}).Error; err != nil {
		t.Fatalf("seed schema-expired row: %v", err)
	}

	expected := uint64(2)
	result, validationErrors, err := service.Restore(ctx, 2, &expected)

	if !errors.Is(err, ErrSnapshotSchemaExpired) {
		t.Fatalf("expected schema expired error, got err=%v result=%#v", err, result)
	}
	if result != nil || validationErrors != nil {
		t.Fatalf("schema expired restore must not carry a result: result=%#v validation=%#v", result, validationErrors)
	}
	record, err := repository.GetRecord(ctx, 1)
	if err != nil || record.Revision != 1 {
		t.Fatalf("failed restore disturbed history revision 1: record=%#v err=%v", record, err)
	}
	var rows int64
	if err := repository.db.Model(&SnapshotRecord{}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("failed restore wrote a revision: %d rows", rows)
	}
}

func TestServiceRestoreRejectsHistoricalConfigFailingCurrentRules(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	ctx := context.Background()
	initial := validRuntimeConfig()
	if _, err := repository.Save(ctx, StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	invalid := validRuntimeConfig()
	invalid.AgentRunner.Model = ""
	payload, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.db.Create(&SnapshotRecord{
		Revision:    2,
		Status:      string(StatusReady),
		Source:      string(SourceUI),
		PayloadJSON: string(payload),
	}).Error; err != nil {
		t.Fatalf("seed rule-expired row: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})

	expected := uint64(2)
	result, validationErrors, err := service.Restore(ctx, 2, &expected)

	if err != nil {
		t.Fatalf("rule failures must come back as field errors, got %v", err)
	}
	if result != nil || !validationErrors.Has("agent_runner.model") {
		t.Fatalf("restore must fail closed with field errors: result=%#v validation=%#v", result, validationErrors)
	}
	latest, err := repository.Latest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Revision != 2 {
		t.Fatalf("failed restore wrote a revision: %#v", latest)
	}
}

func TestServiceRestoreRequiresExpectedRevision(t *testing.T) {
	repository := newConfigurationRepositoryTestDB(t)
	initial := validRuntimeConfig()
	if _, err := repository.Save(context.Background(), StatusReady, SourceUI, initial); err != nil {
		t.Fatalf("seed revision 1: %v", err)
	}
	service := NewService(repository, Resolution{Mode: ModeReady, Prefill: initial, Snapshot: mustLatest(t, repository)})

	result, validationErrors, err := service.Restore(context.Background(), 1, nil)

	if !errors.Is(err, ErrExpectedRevisionRequired) {
		t.Fatalf("expected expected-revision-required, got err=%v result=%#v", err, result)
	}
	if result != nil || validationErrors != nil {
		t.Fatalf("missing expected revision must not carry a result: result=%#v validation=%#v", result, validationErrors)
	}
}

func mustLatest(t *testing.T, repository *Repository) *Snapshot {
	t.Helper()
	snapshot, err := repository.Latest(context.Background())
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	return snapshot
}

func newReadyConfigurationService(t *testing.T) *Service {
	t.Helper()
	repository := newConfigurationRepositoryTestDB(t)
	runtimeConfig := validRuntimeConfig()
	snapshot, err := repository.Save(context.Background(), StatusReady, SourceUI, runtimeConfig)
	if err != nil {
		t.Fatalf("seed runtime configuration: %v", err)
	}
	return NewService(repository, Resolution{Mode: ModeReady, Prefill: runtimeConfig, Snapshot: snapshot})
}

func stringPointer(value string) *string {
	return &value
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}
