-- Migration 002: pending pointer retry table (SQLite only)
-- Stores circular-buffer slot indices that failed to fetch due to network errors.
-- On the next sync the collector retries these specific slots before doing the
-- normal delta fetch.  Entries past their DEADLINE are silently discarded
-- (the ROC circular buffer has 840 slots, so data is gone after ~35 days).

CREATE TABLE IF NOT EXISTS PENDING_POINTERS (
    TASK_KEY    TEXT    NOT NULL,
    PTR_INDEX   INTEGER NOT NULL,
    EXPECTED_TS TEXT    NOT NULL,   -- approximate RFC3339 timestamp of this slot
    DEADLINE    TEXT    NOT NULL,   -- retry until this UTC timestamp (created_at + 840h)
    RETRY_COUNT INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (TASK_KEY, PTR_INDEX)
);
