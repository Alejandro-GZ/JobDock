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

type ObservabilityManifestUpdate struct {
	SourcesAdded []string `json:"sources_added"`
	PhasesAdded  []string `json:"phases_added"`
}

func (s *Store) DeclareObservableSources(ctx context.Context, jobID, attemptID string, sources []domain.ObservableSourceDeclaration) error {
	_, err := s.ApplyObservabilityManifest(ctx, jobID, attemptID, sources, nil)
	return err
}

// ApplyObservabilityManifest extends schema atomically. It never inserts
// samples, observations, cursors, or synthetic timestamps into history.
func (s *Store) ApplyObservabilityManifest(ctx context.Context, jobID, attemptID string, sources []domain.ObservableSourceDeclaration, phases []domain.ObservabilityPhaseDeclaration) (ObservabilityManifestUpdate, error) {
	update := ObservabilityManifestUpdate{SourcesAdded: []string{}, PhasesAdded: []string{}}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return update, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().UnixMilli()
	for _, phase := range phases {
		added, phaseErr := declareObservabilityPhase(ctx, tx, jobID, attemptID, phase, now)
		if phaseErr != nil {
			return update, phaseErr
		}
		if added {
			update.PhasesAdded = append(update.PhasesAdded, phase.ID)
		}
	}
	for _, source := range sources {
		added, sourceErr := declareObservableSource(ctx, tx, jobID, attemptID, source, now)
		if sourceErr != nil {
			return update, sourceErr
		}
		if added {
			update.SourcesAdded = append(update.SourcesAdded, source.Name)
		}
	}
	if len(update.SourcesAdded) > 0 || len(update.PhasesAdded) > 0 {
		payload, marshalErr := json.Marshal(update)
		if marshalErr != nil {
			return update, marshalErr
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO job_events(job_id,attempt_id,sequence,type,status,payload_json,created_at) VALUES(?,?,COALESCE((SELECT MAX(sequence) FROM job_events WHERE job_id=?),0)+1,'observability_manifest_updated',(SELECT status FROM jobs WHERE id=?),?,?)`, jobID, attemptID, jobID, jobID, payload, formatTime(time.Now().UTC())); err != nil {
			return update, err
		}
	}
	return update, tx.Commit()
}

func declareObservabilityPhase(ctx context.Context, tx *sql.Tx, jobID, attemptID string, phase domain.ObservabilityPhaseDeclaration, now int64) (bool, error) {
	metadata := ""
	if len(phase.Metadata) > 0 {
		encoded, err := json.Marshal(phase.Metadata)
		if err != nil {
			return false, err
		}
		metadata = string(encoded)
	}
	var currentName, currentMetadata sql.NullString
	var currentOrder sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT name,position,metadata_json FROM job_observability_phases WHERE attempt_id=? AND phase_id=?`, attemptID, phase.ID).Scan(&currentName, &currentOrder, &currentMetadata)
	if err == nil {
		orderMatches := phase.Order == nil && !currentOrder.Valid || phase.Order != nil && currentOrder.Valid && int64(*phase.Order) == currentOrder.Int64
		if currentName.String != phase.Name || !orderMatches || currentMetadata.String != metadata {
			return false, ErrObservableDeclarationConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO job_observability_phases(job_id,attempt_id,phase_id,name,position,metadata_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, jobID, attemptID, phase.ID, nullableString(phase.Name), phase.Order, nullableString(metadata), now, now)
	return true, mapConstraint(err)
}

func declareObservableSource(ctx context.Context, tx *sql.Tx, jobID, attemptID string, source domain.ObservableSourceDeclaration, now int64) (bool, error) {
	tags, err := encodeMetricTags(source.Tags)
	if err != nil {
		return false, err
	}
	metadata := ""
	if len(source.Metadata) > 0 {
		encoded, marshalErr := json.Marshal(source.Metadata)
		if marshalErr != nil {
			return false, marshalErr
		}
		metadata = string(encoded)
	}
	if err = compatibleObservedDescriptor(ctx, tx, attemptID, source, tags, metadata); err != nil {
		return false, err
	}
	var currentType string
	var currentUnit, currentTags, currentMetadata, currentPhase, currentMilestone sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT source_type,unit,tags_json,metadata_json,phase,milestone FROM job_observability_manifest_sources WHERE attempt_id=? AND name=?`, attemptID, source.Name).Scan(&currentType, &currentUnit, &currentTags, &currentMetadata, &currentPhase, &currentMilestone)
	if err == nil {
		if currentType != source.Type || currentUnit.String != source.Unit || currentTags.String != tags || currentMetadata.String != metadata || currentPhase.String != source.Phase || currentMilestone.String != source.Milestone {
			return false, ErrObservableDeclarationConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO job_observability_manifest_sources(job_id,attempt_id,source_type,name,unit,tags_json,metadata_json,phase,milestone,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, jobID, attemptID, source.Type, source.Name, nullableString(source.Unit), nullableString(tags), nullableString(metadata), nullableString(source.Phase), nullableString(source.Milestone), now, now)
	return true, mapConstraint(err)
}

func compatibleObservedDescriptor(ctx context.Context, tx *sql.Tx, attemptID string, source domain.ObservableSourceDeclaration, tags, metadata string) error {
	var unit, observedMetadata, observedTags sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT unit,metadata_json,tags_json FROM job_metric_descriptors WHERE attempt_id=? AND name=?`, attemptID, source.Name).Scan(&unit, &observedMetadata, &observedTags)
	if err == nil {
		if source.Type != "metric" || source.Unit != "" && unit.Valid && source.Unit != unit.String || metadata != "" && observedMetadata.Valid && metadata != observedMetadata.String || tags != "" && observedTags.Valid && tags != observedTags.String {
			return ErrObservableDeclarationConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind FROM job_rich_observable_descriptors WHERE attempt_id=? AND name=?`, attemptID, source.Name)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		if err = rows.Scan(&kind); err != nil {
			return err
		}
		if kind != source.Type {
			return ErrObservableDeclarationConflict
		}
	}
	return rows.Err()
}

func ensureDeclaredSourceType(ctx context.Context, tx *sql.Tx, attemptID, name, expected string) error {
	var declared string
	err := tx.QueryRowContext(ctx, `SELECT source_type FROM job_observability_manifest_sources WHERE attempt_id=? AND name=?`, attemptID, name).Scan(&declared)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if declared != expected {
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

func (s *Store) ObservabilityPhases(ctx context.Context, jobID, attemptID string) ([]domain.ObservabilityPhaseDeclaration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT phase_id,COALESCE(name,''),position,COALESCE(metadata_json,'') FROM job_observability_phases WHERE job_id=? AND attempt_id=? ORDER BY position IS NULL,position,phase_id LIMIT 128`, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.ObservabilityPhaseDeclaration, 0)
	for rows.Next() {
		var item domain.ObservabilityPhaseDeclaration
		var position sql.NullInt64
		var metadata string
		if err = rows.Scan(&item.ID, &item.Name, &position, &metadata); err != nil {
			return nil, err
		}
		if position.Valid {
			value := int(position.Int64)
			item.Order = &value
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
