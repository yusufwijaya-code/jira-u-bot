-- One-time data steps that must run exactly once, even when the step
-- that created their column is not re-run (Tech Spec "Notifikasi
-- Perubahan Status Tiket": carrying the old toggles into notifyStatusChange).
CREATE TABLE IF NOT EXISTS ms_migration_marker (
  name        VARCHAR(100) NOT NULL PRIMARY KEY,
  appliedDate DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
