package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// ObservableDescriptors returns a bounded attempt-scoped source catalog. It
// reads descriptor tables only; sample and observation history is never loaded.
func (s *Store) ObservableDescriptors(ctx context.Context, jobID, attemptID string) ([]MetricDescriptor, error) {
	metrics, err := s.MetricDescriptors(ctx, jobID, attemptID, nil)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind,name,subtype,unit,tags_json,metadata_json FROM job_rich_observable_descriptors WHERE job_id=? AND attempt_id=? ORDER BY kind,name LIMIT 256`, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	observed := append([]MetricDescriptor(nil), metrics...)
	for rows.Next() {
		item := MetricDescriptor{Observed: true}
		var subtype, unit, tags, metadata sql.NullString
		if err = rows.Scan(&item.Type, &item.Name, &subtype, &unit, &tags, &metadata); err != nil {
			return nil, err
		}
		item.Subtype, item.Unit = subtype.String, unit.String
		if tags.Valid {
			_ = json.Unmarshal([]byte(tags.String), &item.Tags)
		}
		if metadata.Valid {
			_ = json.Unmarshal([]byte(metadata.String), &item.Metadata)
		}
		observed = append(observed, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	declared, err := s.DeclaredObservableSources(ctx, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	return mergeObservableDescriptors(observed, declared), nil
}
