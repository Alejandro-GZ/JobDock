package store

import "context"

// ObservableDescriptors returns a bounded attempt-scoped source catalog. It
// reads descriptor tables only; sample and observation history is never loaded.
func (s *Store) ObservableDescriptors(ctx context.Context, jobID, attemptID string) ([]MetricDescriptor, error) {
	metrics, err := s.MetricDescriptors(ctx, jobID, attemptID, nil)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind,name FROM job_rich_observable_descriptors WHERE job_id=? AND attempt_id=? ORDER BY kind,name LIMIT 256`, jobID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := append([]MetricDescriptor(nil), metrics...)
	for rows.Next() {
		var item MetricDescriptor
		if err = rows.Scan(&item.Type, &item.Name); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
