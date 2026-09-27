package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"opencode-go-manager/internal/model"
)

func TestAutoConnectionSurvivesReopenAndExplicitClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAutoConnection(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("new DB error %v", err)
	}
	want := model.AutoConnection{URL: "https://auto.example/base", Token: "private-token"}
	if err := s.SaveAutoConnection(want); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetAutoConnection()
	if err != nil || got != want {
		t.Fatalf("%+v %v", got, err)
	}
	if err := s.SaveAutoConnection(model.AutoConnection{}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetAutoConnection()
	if err != nil || got.URL != "" || got.Token != "" {
		t.Fatalf("clear lost: %+v %v", got, err)
	}
}
