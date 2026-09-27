package gomodel

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"
)

const (
	DocumentURL      = "https://docs.cline.bot/getting-started/clinepass#models"
	RefreshInterval  = time.Hour
	refreshRetry     = time.Minute
	refreshTimeout   = 20 * time.Second
	maxDocumentBytes = 8 << 20
)

var ErrRefreshInProgress = errors.New("model catalog refresh already in progress")

type Snapshot struct {
	Models       []Info `json:"models"`
	FetchedAt    int64  `json:"fetched_at"`
	ETag         string `json:"etag"`
	LastModified string `json:"last_modified"`
}

type Cache interface {
	LoadModelCatalog() (Snapshot, error)
	SaveModelCatalog(Snapshot) error
}

type SyncStatus struct {
	Source        string `json:"source"`
	IntervalSec   int64  `json:"interval_sec"`
	Syncing       bool   `json:"syncing"`
	Origin        string `json:"origin"`
	LastSuccessAt int64  `json:"last_success_at"`
	LastAttemptAt int64  `json:"last_attempt_at"`
	LastError     string `json:"last_error"`
	ModelCount    int    `json:"model_count"`
}

type Syncer struct {
	cache   Cache
	client  *http.Client
	catalog *Catalog
	url     string

	mu       sync.Mutex
	status   SyncStatus
	snapshot Snapshot
	running  bool

	// Durations are fields so the scheduler can be tested without wall-clock delays.
	interval time.Duration
	retry    time.Duration
}

// NewSyncer restores the last known good catalog before any network requests.
func NewSyncer(cache Cache, client *http.Client, catalog *Catalog) *Syncer {
	if client == nil {
		client = &http.Client{Timeout: refreshTimeout}
	}
	if catalog == nil {
		catalog = DefaultCatalog()
	}
	s := &Syncer{
		cache: cache, client: client, catalog: catalog, url: DocumentURL,
		interval: RefreshInterval, retry: refreshRetry,
		status: SyncStatus{Source: DocumentURL, IntervalSec: int64(RefreshInterval / time.Second), Origin: "builtin"},
	}
	if cache == nil {
		return s
	}
	snapshot, err := cache.LoadModelCatalog()
	if errors.Is(err, sql.ErrNoRows) {
		return s
	}
	if err == nil && snapshot.FetchedAt <= 0 {
		err = errors.New("invalid cached catalog timestamp")
	}
	if err == nil {
		err = catalog.Replace(snapshot.Models)
	}
	if err != nil {
		s.status.LastError = "load model catalog cache: " + err.Error()
		return s
	}
	snapshot.Models = catalog.All()
	s.snapshot = snapshot
	s.status.Origin = "cache"
	s.status.LastSuccessAt = snapshot.FetchedAt
	return s
}

func (s *Syncer) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	status.ModelCount = len(s.catalog.All())
	return status
}

// Refresh leaves both the active catalog and its validators intact on failure.
func (s *Syncer) Refresh(ctx context.Context) (err error) {
	s.mu.Lock()
	if s.status.Syncing {
		s.mu.Unlock()
		return ErrRefreshInProgress
	}
	s.status.Syncing = true
	s.status.LastAttemptAt = time.Now().Unix()
	previous := s.snapshot
	previous.Models = append([]Info(nil), previous.Models...)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.status.Syncing = false
		if err != nil {
			s.status.LastError = err.Error()
		}
		s.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return fmt.Errorf("create model document request: %w", err)
	}
	req.Header.Set("Accept", "text/html, application/xhtml+xml")
	req.Header.Set("User-Agent", "clinepass-manager/model-catalog")
	if len(previous.Models) > 0 {
		if previous.ETag != "" {
			req.Header.Set("If-None-Match", previous.ETag)
		}
		if previous.LastModified != "" {
			req.Header.Set("If-Modified-Since", previous.LastModified)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch model document: %w", err)
	}
	defer resp.Body.Close()

	next := Snapshot{FetchedAt: time.Now().Unix()}
	switch resp.StatusCode {
	case http.StatusNotModified:
		if len(previous.Models) == 0 || (previous.ETag == "" && previous.LastModified == "") {
			return errors.New("model document returned 304 without a validated cached catalog")
		}
		next.Models, next.ETag, next.LastModified = previous.Models, previous.ETag, previous.LastModified
	case http.StatusOK:
		if contentType := resp.Header.Get("Content-Type"); contentType != "" {
			mediaType, _, parseErr := mime.ParseMediaType(contentType)
			if parseErr != nil || mediaType != "text/html" && mediaType != "application/xhtml+xml" {
				return errors.New("model document response is not HTML")
			}
		}
		if resp.ContentLength > maxDocumentBytes {
			return errors.New("model document exceeds size limit")
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxDocumentBytes+1))
		if readErr != nil {
			return fmt.Errorf("read model document: %w", readErr)
		}
		if len(body) > maxDocumentBytes {
			return errors.New("model document exceeds size limit")
		}
		next.Models, err = ParseDocument(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("parse model document: %w", err)
		}
	default:
		return fmt.Errorf("model document returned HTTP %d", resp.StatusCode)
	}
	if value := resp.Header.Get("ETag"); value != "" {
		next.ETag = value
	}
	if value := resp.Header.Get("Last-Modified"); value != "" {
		next.LastModified = value
	}
	// Validate before persistence as well as before publishing the list.
	next.Models, _, err = validateModels(next.Models)
	if err != nil {
		return fmt.Errorf("validate model document: %w", err)
	}
	if s.cache != nil {
		cached := next
		cached.Models = append([]Info(nil), next.Models...)
		if err = s.cache.SaveModelCatalog(cached); err != nil {
			return fmt.Errorf("save model catalog cache: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Validation above guarantees this publication cannot fail.
	_ = s.catalog.Replace(next.Models)
	s.snapshot = next
	s.status.Origin = "remote"
	s.status.LastSuccessAt = next.FetchedAt
	s.status.LastError = ""
	return nil
}

// Run blocks until cancellation. Call it in a goroutine owned by the server.
func (s *Syncer) Run(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	for ctx.Err() == nil {
		delay := s.interval
		if err := s.Refresh(ctx); err != nil {
			delay = s.retry
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
