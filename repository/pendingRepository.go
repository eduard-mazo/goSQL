package repository

import (
	"context"
	"fmt"
	"time"

	"goSQL/db"
)

// PendingPointer is a circular-buffer slot that errored during a sync and
// should be retried on the next run.
type PendingPointer struct {
	TaskKey    string
	PtrIndex   int
	ExpectedTS time.Time // approximate timestamp of the slot when it failed
	Deadline   time.Time // stop retrying after this (first failure + 840 h)
	RetryCount int
}

// PendingRepository manages PENDING_POINTERS (SQLite only).
type PendingRepository struct {
	db *db.DB
}

func NewPendingRepository(database *db.DB) *PendingRepository {
	return &PendingRepository{db: database}
}

// LoadForTask returns all active (non-expired) pending pointers for a task.
func (r *PendingRepository) LoadForTask(ctx context.Context, taskKey string) ([]PendingPointer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT PTR_INDEX, EXPECTED_TS, DEADLINE, RETRY_COUNT
		 FROM PENDING_POINTERS
		 WHERE TASK_KEY = ? AND DEADLINE > ?`,
		taskKey, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("PendingRepo.LoadForTask: %w", err)
	}
	defer rows.Close()

	var result []PendingPointer
	for rows.Next() {
		var pp PendingPointer
		var expStr, dlStr string
		if err := rows.Scan(&pp.PtrIndex, &expStr, &dlStr, &pp.RetryCount); err != nil {
			return nil, fmt.Errorf("PendingRepo scan: %w", err)
		}
		pp.TaskKey = taskKey
		t, err := time.Parse(time.RFC3339, expStr)
		if err != nil {
			return nil, fmt.Errorf("PendingRepo parse EXPECTED_TS %q: %w", expStr, err)
		}
		pp.ExpectedTS = t.Local()
		t2, err := time.Parse(time.RFC3339, dlStr)
		if err != nil {
			return nil, fmt.Errorf("PendingRepo parse DEADLINE %q: %w", dlStr, err)
		}
		pp.Deadline = t2.Local()
		result = append(result, pp)
	}
	return result, rows.Err()
}

// Upsert inserts a new pending pointer or increments RETRY_COUNT on conflict.
// DEADLINE and EXPECTED_TS are only set on the first insert; re-failures
// preserve the original values so the deadline is not extended.
func (r *PendingRepository) Upsert(ctx context.Context, pp PendingPointer) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO PENDING_POINTERS (TASK_KEY, PTR_INDEX, EXPECTED_TS, DEADLINE, RETRY_COUNT)
		 VALUES (?, ?, ?, ?, 0)
		 ON CONFLICT(TASK_KEY, PTR_INDEX) DO UPDATE SET RETRY_COUNT = RETRY_COUNT + 1`,
		pp.TaskKey, pp.PtrIndex,
		pp.ExpectedTS.UTC().Format(time.RFC3339),
		pp.Deadline.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("PendingRepo.Upsert task=%s ptr=%d: %w", pp.TaskKey, pp.PtrIndex, err)
	}
	return nil
}

// Delete removes a pending pointer (called after successful recovery or expiry).
func (r *PendingRepository) Delete(ctx context.Context, taskKey string, ptrIndex int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM PENDING_POINTERS WHERE TASK_KEY = ? AND PTR_INDEX = ?`,
		taskKey, ptrIndex)
	if err != nil {
		return fmt.Errorf("PendingRepo.Delete task=%s ptr=%d: %w", taskKey, ptrIndex, err)
	}
	return nil
}

// PurgeExpired removes all entries past their deadline across all tasks.
func (r *PendingRepository) PurgeExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM PENDING_POINTERS WHERE DEADLINE <= ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("PendingRepo.PurgeExpired: %w", err)
	}
	return nil
}
