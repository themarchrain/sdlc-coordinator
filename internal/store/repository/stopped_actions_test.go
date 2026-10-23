package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

func TestStoppedWritebackRowFailureDoesNotStarveLaterRows(t *testing.T) {
	db, repo, task := stopFixture(t, model.TaskStatusInterrupted)
	if err := store.Migrate(db, &model.Writeback{}); err != nil {
		t.Fatal(err)
	}
	first := model.Writeback{TaskID: task.TaskID, DedupeKey: "first", Kind: "create_issue"}
	second := model.Writeback{TaskID: task.TaskID, DedupeKey: "second", Kind: "create_issue"}
	for _, row := range []*model.Writeback{&first, &second} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	fault := errors.New("temporary first-row failure")
	if err := db.Callback().Update().After("gorm:update").Register("test:first_writeback_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "writebacks" {
			for _, v := range tx.Statement.Vars {
				if id, ok := v.(uint); ok && id == first.ID {
					tx.AddError(fault)
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("test:first_writeback_failure") })
	err := repo.SettleStoppedWritebacks(context.Background(), 20)
	if !errors.Is(err, fault) || !strings.Contains(err.Error(), first.WritebackID) {
		t.Errorf("missing failing-row evidence: %v", err)
	}
	if err := db.First(&second, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if second.Status != model.WritebackStatusSkipped {
		t.Fatalf("healthy later writeback starved: %+v", second)
	}
}
