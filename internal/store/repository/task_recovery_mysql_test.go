package repository

import (
	"context"
	"sync"
	"testing"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func TestRecoveryAcceptMySQLOneWinner(t *testing.T) {
	adminDSN := storetest.RequireMySQL(t)
	dsn := storetest.NewDatabase(t, adminDSN, "recovery_accept")
	first, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, db := range []*gorm.DB{first, second} {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
	})
	if err := store.Migrate(first, &model.Task{}, &model.Run{}, &model.TaskRecovery{}, &model.StopRequest{}); err != nil {
		t.Fatal(err)
	}
	task := recoveryAcceptTask("task-recovery-mysql-race")
	if err := first.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	proof := recoveryProof(task)
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for index, db := range []*gorm.DB{first, second} {
		wg.Add(1)
		go func(index int, db *gorm.DB) {
			defer wg.Done()
			<-start
			command := recoveryCommand(task, "request-mysql-race-"+string(rune('a'+index)), "hash-mysql-race-"+string(rune('a'+index)))
			_, _, err := NewTaskRecoveryRepository(db).Accept(context.Background(), task.TaskID, "member-mysql", command, proof)
			errs <- err
		}(index, db)
	}
	close(start)
	wg.Wait()
	close(errs)
	wins := 0
	for err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("successful MySQL recovery accepts = %d, want 1", wins)
	}
}
