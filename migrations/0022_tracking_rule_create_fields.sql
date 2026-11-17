-- Aturan Board › "Field form Buat Card": per issue type id, the OPTIONAL
-- createmeta fields Buat Card shows ({"10206": ["duedate", ...]}).
-- NULL/empty = only the fields Jira requires.
ALTER TABLE ms_tracking_rule
  ADD COLUMN createFields JSON NULL COMMENT 'issueTypeId -> optional field ids shown on Buat Card';
