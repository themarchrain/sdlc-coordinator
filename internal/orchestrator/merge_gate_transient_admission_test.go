package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/repository"
)

// Keep admission, lease, evidence, audit and notification persistence real;
// only the external code host is replaced by a controllable client.
func transientMergeAdmissionFixture(t *testing.T, mergeErr error) (*gorm.DB, *Orchestrator, *model.Task, *fakeGateClient) {
	t.Helper()
	db, orch, _, _ := externalAdmissionFixture(t)
	if err := store.Migrate(db, &model.MergeGateAudit{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	artifacts := repository.NewArtifactRepository(db)
	verifier := gateFixture(t, ctx, repository.NewTaskRepository(db), artifacts)
	runID := "verify-run"
	if err := db.Create(&model.Run{RunID: runID, TaskID: verifier.TaskID, WorkerRole: verifier.Role, Status: model.RunStatusSucceeded}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(verifier).Update("current_run_id", runID).Error; err != nil {
		t.Fatal(err)
	}
	client := &fakeGateClient{liveHeadSHA: gateHeadSHA, mergeErr: mergeErr}
	orch.mergeAudits = repository.NewMergeGateAuditRepository(db)
	orch.verificationArtifacts = artifacts
	orch.autoMergeEnabled = func(context.Context) bool { return true }
	orch.repositoryLookup = &gateRepositoryLookup{repo: &registeredrepo.RegisteredRepository{RepositoryID: fixtureRepositoryID, Provider: "gitea", PathWithNamespace: "group/project", Status: registeredrepo.StatusEnabled}}
	orch.mergeGateClients = func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
		return client, client, nil
	}
	return db, orch, verifier, client
}

func TestAutoMergeAmbiguousTransientRejectionDoesNotResend(t *testing.T) {
	for _, mergeErr := range []error{
		codehost.NewAmbiguousError(codehost.NewTransientRejection(errors.New("merge accepted; readback HTTP 429"))),
		codehost.NewTransientRejection(codehost.NewAmbiguousError(errors.New("unverified merge outcome"))),
	} {
		t.Run(mergeErr.Error(), func(t *testing.T) {
			db, orch, verifier, client := transientMergeAdmissionFixture(t, mergeErr)
			ctx := context.Background()
			first, err := orch.RunAutoMergeGate(ctx, 20)
			if err != nil || len(first) != 1 || first[0].Decision != MergeGateDecisionSkipped {
				t.Fatalf("first=%+v err=%v", first, err)
			}
			var admission model.ActionAdmission
			if err := db.Where("task_id = ? AND kind = ?", verifier.TaskID, model.ActionAutoMerge).First(&admission).Error; err != nil || admission.NotSentAt != nil {
				t.Fatalf("ambiguous outcome marked unapplied: %+v err=%v", admission, err)
			}
			client.mergeErr = nil
			second, err := orch.RunAutoMergeGate(ctx, 20)
			if err != nil || len(second) != 1 || second[0].Decision != MergeGateDecisionFallback || client.mergeCalls != 1 {
				t.Fatalf("ambiguous merge resent: second=%+v calls=%d err=%v", second, client.mergeCalls, err)
			}
		})
	}
}

type failingMergeNonDeliveryProof struct {
	*repository.ActionAdmissionRepository
	err error
}

func (r *failingMergeNonDeliveryProof) ConfirmNotSent(context.Context, *model.ActionAdmission) error {
	return r.err
}

func TestAutoMergeTransientRejectionProofFailureDoesNotResend(t *testing.T) {
	db, orch, verifier, client := transientMergeAdmissionFixture(t, codehost.NewTransientRejection(errors.New("HTTP 429 not applied")))
	proofErr := errors.New("non-delivery proof storage unavailable")
	orch.actionAdmissions = &failingMergeNonDeliveryProof{ActionAdmissionRepository: repository.NewActionAdmissionRepository(db), err: proofErr}
	ctx := context.Background()
	first, err := orch.RunAutoMergeGate(ctx, 20)
	if err != nil || len(first) != 1 || first[0].Decision != MergeGateDecisionSkipped || !strings.Contains(first[0].Error, proofErr.Error()) {
		t.Fatalf("proof failure not surfaced: first=%+v err=%v", first, err)
	}
	var admission model.ActionAdmission
	if err := db.Where("task_id = ? AND kind = ?", verifier.TaskID, model.ActionAutoMerge).First(&admission).Error; err != nil || admission.NotSentAt != nil {
		t.Fatalf("failed proof marked unapplied: %+v err=%v", admission, err)
	}
	client.mergeErr = nil
	second, err := orch.RunAutoMergeGate(ctx, 20)
	if err != nil || len(second) != 1 || second[0].Decision != MergeGateDecisionFallback || client.mergeCalls != 1 {
		t.Fatalf("merge resent without durable proof: second=%+v calls=%d err=%v", second, client.mergeCalls, err)
	}
}

func TestAutoMergeTransientRejectionRetriesWithNewAdmission(t *testing.T) {
	db, orch, verifier, client := transientMergeAdmissionFixture(t, codehost.NewTransientRejection(errors.New("merge checking; try again later")))
	ctx := context.Background()
	first, err := orch.RunAutoMergeGate(ctx, 20)
	if err != nil || len(first) != 1 || first[0].Decision != MergeGateDecisionSkipped || client.mergeCalls != 1 {
		t.Fatalf("first=%+v calls=%d err=%v", first, client.mergeCalls, err)
	}
	var previous model.ActionAdmission
	if err := db.Where("task_id = ? AND kind = ?", verifier.TaskID, model.ActionAutoMerge).First(&previous).Error; err != nil {
		t.Fatal(err)
	}
	if previous.NotSentAt == nil || previous.Attempt != 1 {
		t.Fatalf("transient rejection left admission unresolved: %+v", previous)
	}
	for _, record := range []any{&model.MergeGateAudit{}, &model.Effect{}} {
		var count int64
		if err := db.Model(record).Where("task_id = ?", verifier.TaskID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("transient rejection created terminal record %T: count=%d err=%v", record, count, err)
		}
	}

	client.mergeErr = nil
	second, err := orch.RunAutoMergeGate(ctx, 20)
	if err != nil || len(second) != 1 || second[0].Decision != MergeGateDecisionMerged || client.mergeCalls != 2 {
		t.Fatalf("retry=%+v calls=%d err=%v", second, client.mergeCalls, err)
	}
	if client.mergeReq.ExpectedHeadSHA != gateHeadSHA {
		t.Fatalf("retry lost evidence head assertion: %+v", client.mergeReq)
	}
	var admissions []model.ActionAdmission
	if err := db.Where("task_id = ? AND kind = ?", verifier.TaskID, model.ActionAutoMerge).Order("attempt").Find(&admissions).Error; err != nil {
		t.Fatal(err)
	}
	if len(admissions) != 2 || admissions[1].Attempt != 2 || admissions[1].NotSentAt != nil {
		t.Fatalf("retry admissions=%+v", admissions)
	}
	var audit model.MergeGateAudit
	if err := db.Where("task_id = ?", verifier.TaskID).First(&audit).Error; err != nil || audit.Decision != MergeGateDecisionMerged || audit.MergedCommitSHA != "merged999" {
		t.Fatalf("merge audit=%+v err=%v", audit, err)
	}
	var messages int64
	if err := db.Model(&model.Effect{}).Where("task_id = ?", verifier.TaskID).Count(&messages).Error; err != nil || messages != 0 {
		t.Fatalf("successful retry generated fallback messages=%d err=%v", messages, err)
	}
	third, err := orch.RunAutoMergeGate(ctx, 20)
	if err != nil || len(third) != 0 || client.mergeCalls != 2 {
		t.Fatalf("terminal merged task resent: third=%+v calls=%d err=%v", third, client.mergeCalls, err)
	}
}
