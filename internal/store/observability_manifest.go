package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

// DeclareObservableSources persists schema only. It never inserts samples,
// observations, cursors, or synthetic timestamps into telemetry history.
func (s *Store) DeclareObservableSources(ctx context.Context, jobID, attemptID string, sources []domain.ObservableSourceDeclaration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().UnixMilli()
	for _, source := range sources {
		tags, err := encodeMetricTags(source.Tags)
		if err != nil {
			return err
		}
		metadata := ""
		if len(source.Metadata) > 0 {
			encoded, marshalErr := json.Marshal(source.Metadata)
			if marshalErr != nil {
				return marshalErr
			}
			metadata = string(encoded)
		}
		if err = compatibleObservedDescriptor(ctx, tx, attemptID, source, tags, metadata); err != nil {
			return err
		}
		var currentUnit, currentTags, currentMetadata, currentPhase, currentMilestone sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT unit,tags_json,metadata_json,phase,milestone FROM job_observability_manifest_sources WHERE attempt_id=? AND source_type=? AND name=?`, attemptID, source.Type, source.Name).Scan(&currentUnit, &currentTags, &currentMetadata, &currentPhase, &currentMilestone)
		if err == nil {
			if currentUnit.String != source.Unit || currentTags.String != tags || currentMetadata.String != metadata || currentPhase.String != source.Phase || currentMilestone.String != source.Milestone {
				return ErrObservableDeclarationConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO job_observability_manifest_sources(job_id,attempt_id,source_type,name,unit,tags_json,metadata_json,phase,milestone,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, jobID, attemptID, source.Type, source.Name, nullableString(source.Unit), nullableString(tags), nullableString(metadata), nullableString(source.Phase), nullableString(source.Milestone), now, now)
		if err != nil {
			return mapConstraint(err)
		}
	}
	return tx.Commit()
}

func compatibleObservedDescriptor(ctx context.Context, tx *sql.Tx, attemptID string, source domain.ObservableSourceDeclaration, tags, metadata string) error {
	if source.Type != "metric" {
		return nil
	}
	var unit, observedMetadata, observedTags sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT unit,metadata_json,tags_json FROM job_metric_descriptors WHERE attempt_id=? AND name=?`, attemptID, source.Name).Scan(&unit, &observedMetadata, &observedTags)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if source.Unit != "" && unit.Valid && source.Unit != unit.String || metadata != "" && observedMetadata.Valid && metadata != observedMetadata.String || tags != "" && observedTags.Valid && tags != observedTags.String {
		return ErrObservableDeclarationConflict
	}
	return nil
}

func (s *Store) DeclaredObservableSources(ctx context.Context, jobID, attemptID string) ([]MetricDescriptor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_type,name,COALESCE(unit,''),COALESCE(tags_json,''),COALESCE(metadata_json,''),COALESCE(phase,''),COALESCE(milestone,'') FROM job_observability_manifest_sources WHERE job_id=? AND attempt_id=? ORDER BY source_type,name LIMIT 256`, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]MetricDescriptor, 0)
	for rows.Next() {
		item := MetricDescriptor{Declared: true}
		var tags, metadata string
		if err = rows.Scan(&item.Type, &item.Name, &item.Unit, &tags, &metadata, &item.Phase, &item.Milestone); err != nil {
			return nil, err
		}
		if tags != "" {
			_ = json.Unmarshal([]byte(tags), &item.Tags)
		}
		if metadata != "" {
			_ = json.Unmarshal([]byte(metadata), &item.Metadata)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func mergeObservableDescriptors(observed, declared []MetricDescriptor) []MetricDescriptor {
	items := make(map[string]MetricDescriptor, len(observed)+len(declared))
	for _, item := range observed {
		items[item.Type+"\x00"+item.Name] = item
	}
	for _, declaration := range declared {
		key := declaration.Type + "\x00" + declaration.Name
		if item, ok := items[key]; ok {
			item.Declared = true
			item.Phase, item.Milestone = declaration.Phase, declaration.Milestone
			if item.Unit == "" {
				item.Unit = declaration.Unit
			}
			if len(item.Tags) == 0 {
				item.Tags = declaration.Tags
			}
			if len(item.Metadata) == 0 {
				item.Metadata = declaration.Metadata
			}
			items[key] = item
		} else {
			items[key] = declaration
		}
	}
	result := make([]MetricDescriptor, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := observableKindOrder(result[i].Type), observableKindOrder(result[j].Type)
		if left != right {
			return left < right
		}
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Name < result[j].Name
	})
	if len(result) > 256 {
		result = result[:256]
	}
	return result
}

func observableKindOrder(kind string) int {
	switch kind {
	case "metric":
		return 0
	case "matrix":
		return 1
	case "progress":
		return 2
	case "resource":
		return 3
	case "log":
		return 4
	default:
		return 5
	}
}

func descriptorHasTags(item MetricDescriptor, required []string) bool {
	available := make(map[string]bool, len(item.Tags))
	for _, tag := range item.Tags {
		available[tag] = true
	}
	for _, tag := range required {
		if !available[tag] {
			return false
		}
	}
	return true
}
