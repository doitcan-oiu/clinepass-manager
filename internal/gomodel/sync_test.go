package gomodel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryCatalogCache struct {
	mu       sync.Mutex
	snapshot Snapshot
	saveErr  error
	loadErr  error
}

func (c *memoryCatalogCache) LoadModelCatalog() (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loadErr != nil {
		return Snapshot{}, c.loadErr
	}
	if c.snapshot.FetchedAt == 0 {
		return Snapshot{}, sql.ErrNoRows
	}
	value := c.snapshot
	value.Models = append([]Info(nil), value.Models...)
	return value, nil
}

func (c *memoryCatalogCache) SaveModelCatalog(value Snapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.saveErr != nil {
		return c.saveErr
	}
	value.Models = append([]Info(nil), value.Models...)
	c.snapshot = value
	return nil
}

func modelDocument(ids ...string) string {
	var html strings.Builder
	html.WriteString(`<html><h2 id="models">Models</h2><p>Current models</p><table><thead><tr><th>Model</th><th>Model ID</th></tr></thead><tbody>`)
	for _, id := range ids {
		fmt.Fprintf(&html, `<tr><td>Model %s</td><td><code>cline-pass/%s</code></td></tr>`, id, id)
	}
	html.WriteString(`</tbody></table><h2 id="pricing">Pricing</h2></html>`)
	return html.String()
}

func testSyncer(t *testing.T, cache Cache, handler http.HandlerFunc) (*Syncer, *Catalog) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	catalog := NewCatalog()
	syncer := NewSyncer(cache, server.Client(), catalog)
	syncer.url = server.URL
	return syncer, catalog
}

func TestSyncRefreshUpdatesCatalogAndRestoresCache(t *testing.T) {
	cache := &memoryCatalogCache{}
	syncer, catalog := testSyncer(t, cache, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			t.Error("request does not ask for HTML")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("ETag", `"revision-1"`)
		fmt.Fprint(w, modelDocument("new-model", "another-model"))
	})
	if got := syncer.Status(); got.Origin != "builtin" || got.ModelCount != 12 {
		t.Fatalf("unexpected initial status: %+v", got)
	}
	if err := syncer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog.Lookup("new-model"); !ok {
		t.Fatal("new model not published")
	}
	if _, ok := catalog.Lookup("glm-5.3"); ok {
		t.Fatal("deleted model still accepted")
	}
	status := syncer.Status()
	if status.Origin != "remote" || status.ModelCount != 2 || status.LastSuccessAt == 0 || status.LastAttemptAt == 0 || status.Syncing || status.LastError != "" {
		t.Fatalf("unexpected refreshed status: %+v", status)
	}
	if status.Source != DocumentURL || status.IntervalSec != 3600 {
		t.Fatalf("incorrect synchronization contract: %+v", status)
	}
	restarted := NewCatalog()
	reloaded := NewSyncer(cache, nil, restarted)
	if got := reloaded.Status(); got.Origin != "cache" || got.ModelCount != 2 || got.LastSuccessAt != status.LastSuccessAt {
		t.Fatalf("cache not restored: %+v", got)
	}
	if _, ok := restarted.Lookup("another-model"); !ok {
		t.Fatal("restart lost cached models")
	}
}

func TestSyncRefreshRetainsCatalogOnFailures(t *testing.T) {
	for _, kind := range []string{"http", "parse", "non-html", "oversize", "save", "unexpected-304"} {
		t.Run(kind, func(t *testing.T) {
			cache := &memoryCatalogCache{}
			if kind == "save" {
				cache.saveErr = errors.New("disk full")
			}
			syncer, catalog := testSyncer(t, cache, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				switch kind {
				case "http":
					w.WriteHeader(http.StatusBadGateway)
				case "parse":
					fmt.Fprint(w, `<html>maintenance</html>`)
				case "non-html":
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, modelDocument("fresh"))
				case "oversize":
					w.Header().Set("Content-Length", fmt.Sprint(maxDocumentBytes+1))
					fmt.Fprint(w, modelDocument("fresh"))
				case "save":
					fmt.Fprint(w, modelDocument("fresh"))
				case "unexpected-304":
					w.WriteHeader(http.StatusNotModified)
				}
			})
			if err := syncer.Refresh(context.Background()); err == nil {
				t.Fatal("refresh unexpectedly succeeded")
			}
			if got := syncer.Status(); got.Origin != "builtin" || got.ModelCount != 12 || got.LastError == "" || got.Syncing || got.LastSuccessAt != 0 {
				t.Fatalf("failure did not preserve status: %+v", got)
			}
			if _, ok := catalog.Lookup("fresh"); ok {
				t.Fatal("failed refresh changed catalog")
			}
			if _, err := cache.LoadModelCatalog(); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("failed refresh overwrote cache")
			}
		})
	}
}

func TestSyncConditionalRefreshAndSaveFailure(t *testing.T) {
	cache := &memoryCatalogCache{snapshot: Snapshot{
		Models: []Info{catalogFixture("cached")}, FetchedAt: 123,
		ETag: `"old-tag"`, LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
	}}
	syncer, catalog := testSyncer(t, cache, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"old-tag"` || r.Header.Get("If-Modified-Since") != "Mon, 01 Jan 2024 00:00:00 GMT" {
			t.Error("missing conditional request validators")
		}
		w.Header().Set("ETag", `"new-tag"`)
		w.WriteHeader(http.StatusNotModified)
	})
	cache.saveErr = errors.New("disk unavailable")
	if err := syncer.Refresh(context.Background()); err == nil {
		t.Fatal("304 should not succeed if persistence fails")
	}
	if syncer.Status().LastSuccessAt != 123 {
		t.Fatal("failed persistence changed last success time")
	}
	cache.saveErr = nil
	if err := syncer.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(catalog.All()) != 1 || catalog.All()[0].ID != "cline-pass/cached" {
		t.Fatal("304 discarded prior models")
	}
	value, _ := cache.LoadModelCatalog()
	if value.ETag != `"new-tag"` || value.FetchedAt <= 123 || value.LastModified == "" {
		t.Fatalf("304 did not update cache correctly: %+v", value)
	}
	if status := syncer.Status(); status.LastError != "" || status.Origin != "remote" {
		t.Fatalf("successful retry did not clear error: %+v", status)
	}
}

func TestSyncIgnoresCorruptCache(t *testing.T) {
	cache := &memoryCatalogCache{snapshot: Snapshot{Models: []Info{{ID: "bad", Name: "Bad"}}, FetchedAt: 12}}
	catalog := NewCatalog()
	syncer := NewSyncer(cache, nil, catalog)
	if status := syncer.Status(); status.Origin != "builtin" || status.ModelCount != 12 || status.LastError == "" {
		t.Fatalf("invalid cache was not rejected: %+v", status)
	}
}

func TestSyncRunRefreshesImmediatelyRetriesAndStops(t *testing.T) {
	var requests atomic.Int32
	observed := make(chan int32, 8)
	syncer, _ := testSyncer(t, &memoryCatalogCache{}, func(w http.ResponseWriter, r *http.Request) {
		count := requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		if count == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			fmt.Fprint(w, modelDocument("fresh"))
		}
		select {
		case observed <- count:
		default:
		}
	})
	syncer.interval = 20 * time.Millisecond
	syncer.retry = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()
	for expected := int32(1); expected <= 3; expected++ {
		select {
		case got := <-observed:
			if got != expected {
				t.Fatalf("request %d, want %d", got, expected)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("periodic refresh did not execute")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run ignored cancellation")
	}
}

func TestSyncConcurrentRefreshDoesNotDuplicateRequest(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	syncer, _ := testSyncer(t, &memoryCatalogCache{}, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, modelDocument("fresh"))
	})
	result := make(chan error, 1)
	go func() { result <- syncer.Refresh(context.Background()) }()
	<-entered
	if !syncer.Status().Syncing {
		t.Fatal("refresh status not visible during request")
	}
	err := syncer.Refresh(context.Background())
	close(release)
	if !errors.Is(err, ErrRefreshInProgress) {
		t.Fatalf("concurrent refresh returned %v", err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
