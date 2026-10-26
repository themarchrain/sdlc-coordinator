package repository

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/model"
)

func TestStopPendingCreateWithoutExecutionProjection(t *testing.T) {
	db, stops, task := stopFixture(t, model.TaskStatusRunning)
	run := &model.Run{RunID: "run", TaskID: task.TaskID, Status: model.RunStatusRunning, RunnerEndpoint: "original-runner"}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("current_run_id", run.RunID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ActionAdmission{AdmissionID: "create-in-flight", TaskID: task.TaskID, RunID: run.RunID, Kind: model.ActionExecutionCreate, ActionKey: run.RunID, Attempt: 1, LeaseOwner: "old", LeaseGeneration: 1}).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := stops.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Stop.DeliveryStatus != model.StopDeliveryPending || decision.Stop.ConfirmationKind != "" || decision.Stop.ExecutionID != run.RunID || decision.Stop.RunnerEndpoint != run.RunnerEndpoint {
		t.Fatalf("lost admitted create: %+v", decision.Stop)
	}
}

func TestStopNoRemoteTargetHasExplicitProof(t *testing.T) {
	_, stops, task := stopFixture(t, model.TaskStatusPending)
	decision, err := stops.Stop(context.Background(), StopTarget{TaskID: task.TaskID}, "admin", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Stop.ConfirmationKind != model.StopConfirmationNoRemoteTarget || decision.Stop.DeliveryStatus != model.StopDeliveryConfirmed {
		t.Fatalf("no-target proof missing: %+v", decision.Stop)
	}
}
