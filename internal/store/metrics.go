package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jobdock/jobdock/internal/domain"
)

func (s *Store) AppendMetricSamples(ctx context.Context, samples []domain.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cursor, err := insertSeriesUpdate(ctx, tx, samples[0].JobID, samples[0].AttemptID, "metrics", samples[0].CapturedAt)
	if err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO job_metric_samples(job_id,attempt_id,name,step,value,captured_at,series_cursor) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, sample := range samples {
		if sample.JobID != samples[0].JobID || sample.AttemptID != samples[0].AttemptID {
			return fmt.Errorf("metric batch spans multiple job attempts")
		}
		if _, err = statement.ExecContext(ctx, sample.JobID, sample.AttemptID, sample.Name, sample.Step, sample.Value, sample.CapturedAt.UTC().UnixMilli(), cursor); err != nil {
			return mapConstraint(err)
		}
	}
	return tx.Commit()
}

func (s *Store) MetricSeries(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int) ([]domain.MetricSeries, bool, error) {
	return s.metricSeries(ctx, jobID, attemptID, names, from, to, resolutionSeconds, limit, nil)
}

func (s *Store) MetricSeriesAt(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int, maxCursor int64) ([]domain.MetricSeries, bool, error) {
	return s.metricSeries(ctx, jobID, attemptID, names, from, to, resolutionSeconds, limit, &maxCursor)
}

func (s *Store) metricSeries(ctx context.Context, jobID, attemptID string, names []string, from, to time.Time, resolutionSeconds, limit int, maxCursor *int64) ([]domain.MetricSeries, bool, error) {
	arguments := []any{jobID, attemptID, from.UTC().UnixMilli(), to.UTC().UnixMilli()}
	nameClause := ""
	if len(names) > 0 {
		placeholders := make([]string, len(names))
		for index, name := range names {
			placeholders[index] = "?"
			arguments = append(arguments, name)
		}
		nameClause = " AND name IN (" + strings.Join(placeholders, ",") + ")"
	}
	cursorClause := ""
	if maxCursor != nil {
		cursorClause = " AND (series_cursor IS NULL OR series_cursor<=?)"
		arguments = append(arguments, *maxCursor)
	}
	var query string
	if resolutionSeconds <= 0 {
		query = `SELECT name,step,value,captured_at,1,COALESCE(series_cursor,0) FROM job_metric_samples WHERE job_id=? AND attempt_id=? AND captured_at>=? AND captured_at<=?` + nameClause + cursorClause + ` ORDER BY name,captured_at,id LIMIT ?`
	} else {
		bucketMilliseconds := int64(resolutionSeconds) * 1000
		query = `SELECT name,MAX(step),AVG(value),(captured_at/?)*?,COUNT(*),MAX(COALESCE(series_cursor,0)) FROM job_metric_samples WHERE job_id=? AND attempt_id=? AND captured_at>=? AND captured_at<=?` + nameClause + cursorClause + ` GROUP BY name,(captured_at/?) ORDER BY name,(captured_at/?) LIMIT ?`
		arguments = append([]any{bucketMilliseconds, bucketMilliseconds}, arguments...)
		arguments = append(arguments, bucketMilliseconds, bucketMilliseconds)
	}
	arguments = append(arguments, limit+1)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	seriesByName := map[string]*domain.MetricSeries{}
	order := make([]string, 0)
	pointCount := 0
	for rows.Next() {
		var name string
		var step sql.NullInt64
		var value float64
		var captured, sampleCount, cursor int64
		if err = rows.Scan(&name, &step, &value, &captured, &sampleCount, &cursor); err != nil {
			return nil, false, err
		}
		item := seriesByName[name]
		if item == nil {
			item = &domain.MetricSeries{Name: name, Points: []domain.MetricPoint{}}
			seriesByName[name] = item
			order = append(order, name)
		}
		var stepValue *int64
		if step.Valid {
			value := step.Int64
			stepValue = &value
		}
		item.Points = append(item.Points, domain.MetricPoint{Cursor: cursor, CapturedAt: time.UnixMilli(captured).UTC(), Step: stepValue, Value: value, SampleCount: sampleCount})
		pointCount++
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := pointCount > limit
	if truncated {
		for index := len(order) - 1; index >= 0 && pointCount > limit; index-- {
			item := seriesByName[order[index]]
			for len(item.Points) > 0 && pointCount > limit {
				item.Points = item.Points[:len(item.Points)-1]
				pointCount--
			}
		}
	}
	result := make([]domain.MetricSeries, 0, len(order))
	for _, name := range order {
		item := seriesByName[name]
		statsCursorClause := ""
		statsArguments := []any{jobID, attemptID, name, from.UTC().UnixMilli(), to.UTC().UnixMilli()}
		if maxCursor != nil {
			statsCursorClause = " AND (series_cursor IS NULL OR series_cursor<=?)"
			statsArguments = append(statsArguments, *maxCursor)
		}
		statsArguments = append(statsArguments, jobID, attemptID, name, from.UTC().UnixMilli(), to.UTC().UnixMilli())
		if maxCursor != nil {
			statsArguments = append(statsArguments, *maxCursor)
		}
		statsQuery := `SELECT MIN(value),MAX(value),COUNT(*),(SELECT value FROM job_metric_samples latest WHERE latest.job_id=? AND latest.attempt_id=? AND latest.name=? AND latest.captured_at>=? AND latest.captured_at<=?` + statsCursorClause + ` ORDER BY latest.captured_at DESC,latest.id DESC LIMIT 1) FROM job_metric_samples WHERE job_id=? AND attempt_id=? AND name=? AND captured_at>=? AND captured_at<=?` + statsCursorClause
		if err = s.db.QueryRowContext(ctx, statsQuery, statsArguments...).Scan(&item.Min, &item.Max, &item.SampleCount, &item.Last); err != nil {
			return nil, false, fmt.Errorf("metric statistics: %w", err)
		}
		result = append(result, *item)
	}
	return result, truncated, nil
}

func (s *Store) AttemptBelongsToJob(ctx context.Context, jobID, attemptID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM job_attempts WHERE id=? AND job_id=?`, attemptID, jobID).Scan(&count)
	return count == 1, err
}
