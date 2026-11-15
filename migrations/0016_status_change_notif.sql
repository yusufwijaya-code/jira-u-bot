-- Tech Spec "Notifikasi Perubahan Status Tiket": the last status the
-- status watcher saw per ticket, to detect changes made in Jira or in Nudge.
CREATE TABLE IF NOT EXISTS tr_issuestatus (
  issueKey      VARCHAR(32)  NOT NULL PRIMARY KEY,
  status        VARCHAR(100) NOT NULL,
  assigneeEmail VARCHAR(255) NOT NULL,
  lastSeenDate  DATETIME     NOT NULL,
  editedDate    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY idx_issuestatus_lastseen (lastSeenDate)
);
