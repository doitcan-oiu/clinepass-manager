package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"opencode-go-manager/internal/gomodel"
)

func TestModelCatalogPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if _, err := st.LoadModelCatalog(); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing cache: %v", err)
	}
	want := gomodel.Snapshot{Models: []gomodel.Info{{ID: "cline-pass/new-model", Name: "New Model", Endpoint: gomodel.EndpointChat}}, FetchedAt: 12345, ETag: `"v2"`, LastModified: "Sun, 27 Sep 2026 01:00:00 GMT"}
	if err := st.SaveModelCatalog(want); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadModelCatalog()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("cache after restart: %+v, %v", got, err)
	}
	if err := st.SaveModelCatalog(gomodel.Snapshot{}); err == nil {
		t.Fatal("empty snapshot replaced valid cache")
	}
	got, err = st.LoadModelCatalog()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("cache changed after invalid save: %+v, %v", got, err)
	}
}
