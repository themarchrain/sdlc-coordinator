package people_test

import (
	"context"
	"testing"

	"sdlc-coordinator/internal/people"
	"sdlc-coordinator/internal/store"
	"sdlc-coordinator/internal/store/storetest"

	"gorm.io/gorm"
)

func openPeopleMySQL(t *testing.T) (*gorm.DB, *people.Repository) {
	t.Helper()
	admin := storetest.RequireMySQL(t)
	db := storetest.OpenDatabase(t, admin, "people_idx")
	return db, people.NewRepository(db)
}

func TestPeopleMigrateIdempotentOnMySQL(t *testing.T) {
	_, repository := openPeopleMySQL(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := repository.Migrate(); err != nil {
		t.Fatalf("second migrate must be idempotent on mysql: %v", err)
	}
}

// TestPeoplePartialUniqueIndexesOnMySQL 锁定草稿感知唯一性的 MySQL 等价语义：
// 存活行受唯一约束（邮箱与 username 大小写不敏感）、软删除行不参与唯一性。
func TestPeoplePartialUniqueIndexesOnMySQL(t *testing.T) {
	ctx := context.Background()
	_, repository := openPeopleMySQL(t)
	if err := repository.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	create := func(email string) *people.Person {
		t.Helper()
		person := &people.Person{DisplayName: email, Email: email, Enabled: true}
		if err := repository.Create(ctx, person, nil); err != nil {
			t.Fatalf("create person %s: %v", email, err)
		}
		return person
	}

	first := create("Dev@Example.com")
	dup := &people.Person{DisplayName: "dup", Email: "dev@example.com", Enabled: true}
	err := repository.Create(ctx, dup, nil)
	if err == nil {
		t.Fatal("case-different duplicate email accepted (NOCASE semantics lost)")
	}
	if !store.IsDuplicateKey(err) {
		t.Fatalf("duplicate email not classified by driver code: %v", err)
	}

	if err := repository.Delete(ctx, first.PersonID); err != nil {
		t.Fatalf("soft delete person: %v", err)
	}
	create("dev@example.com")

	owner := &people.Person{DisplayName: "owner@example.com", Email: "owner@example.com", Enabled: true}
	identity := people.CodeHostIdentity{Provider: "github", CodeHostBaseURL: "https://github.com", ExternalUserID: "42", Username: "DevUser"}
	if err := repository.Create(ctx, owner, []people.CodeHostIdentity{identity}); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	other := &people.Person{DisplayName: "other@example.com", Email: "other@example.com", Enabled: true}
	if err := repository.Create(ctx, other, []people.CodeHostIdentity{
		{Provider: "github", CodeHostBaseURL: "https://github.com", ExternalUserID: "42", Username: "SomeoneElse"},
	}); err == nil {
		t.Fatal("duplicate (provider, base_url, external_user_id) accepted")
	}
	third := &people.Person{DisplayName: "third@example.com", Email: "third@example.com", Enabled: true}
	if err := repository.Create(ctx, third, []people.CodeHostIdentity{
		{Provider: "github", CodeHostBaseURL: "https://github.com", ExternalUserID: "77", Username: "devuser"},
	}); err == nil {
		t.Fatal("duplicate identity username with different case accepted (NOCASE semantics lost)")
	}

	if err := repository.Delete(ctx, owner.PersonID); err != nil {
		t.Fatalf("soft delete identity owner: %v", err)
	}
	fourth := &people.Person{DisplayName: "fourth@example.com", Email: "fourth@example.com", Enabled: true}
	if err := repository.Create(ctx, fourth, []people.CodeHostIdentity{identity}); err != nil {
		t.Fatalf("re-create identity after soft delete: %v", err)
	}
}
