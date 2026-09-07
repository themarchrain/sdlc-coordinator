package store

import "testing"

func TestOpenSQLite(t *testing.T) {
	db, err := Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if db == nil {
		t.Fatal("expected db instance")
	}
}

