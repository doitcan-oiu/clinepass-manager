package store

import (
	"encoding/json"
	"fmt"

	"opencode-go-manager/internal/gomodel"
)

func (s *Store) ensureModelCatalog() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS model_catalog (
		id INTEGER PRIMARY KEY CHECK(id=1),
		source TEXT NOT NULL,
		models_json TEXT NOT NULL,
		fetched_at INTEGER NOT NULL,
		etag TEXT NOT NULL DEFAULT '',
		last_modified TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

func (s *Store) LoadModelCatalog() (gomodel.Snapshot, error) {
	var snapshot gomodel.Snapshot
	if err := s.ensureModelCatalog(); err != nil {
		return snapshot, err
	}
	var raw string
	err := s.db.QueryRow(`SELECT models_json,fetched_at,etag,last_modified FROM model_catalog WHERE id=1 AND source=?`, gomodel.DocumentURL).
		Scan(&raw, &snapshot.FetchedAt, &snapshot.ETag, &snapshot.LastModified)
	if err != nil {
		return snapshot, err
	}
	if err := json.Unmarshal([]byte(raw), &snapshot.Models); err != nil {
		return gomodel.Snapshot{}, fmt.Errorf("读取模型目录缓存失败: %w", err)
	}
	return snapshot, nil
}

func (s *Store) SaveModelCatalog(snapshot gomodel.Snapshot) error {
	if snapshot.FetchedAt <= 0 || len(snapshot.Models) == 0 {
		return fmt.Errorf("模型目录缓存不能为空或缺少同步时间")
	}
	raw, err := json.Marshal(snapshot.Models)
	if err != nil {
		return err
	}
	if err := s.ensureModelCatalog(); err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO model_catalog(id,source,models_json,fetched_at,etag,last_modified) VALUES(1,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET source=excluded.source,models_json=excluded.models_json,fetched_at=excluded.fetched_at,etag=excluded.etag,last_modified=excluded.last_modified`,
		gomodel.DocumentURL, string(raw), snapshot.FetchedAt, snapshot.ETag, snapshot.LastModified)
	return err
}
