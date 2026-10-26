CREATE TABLE IF NOT EXISTS ms_user (
  userID INTEGER PRIMARY KEY AUTOINCREMENT,
  userName TEXT NOT NULL,
  email TEXT NOT NULL,
  flagActive INTEGER NOT NULL DEFAULT 1,
  createdBy TEXT NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  editedBy TEXT,
  editedDate DATETIME,
  UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS tr_issuesession (
  issueSessionID INTEGER PRIMARY KEY AUTOINCREMENT,
  issueKey TEXT NOT NULL,
  projectKey TEXT NOT NULL,
  storyPoints REAL NOT NULL,
  accumulatedSeconds INTEGER NOT NULL DEFAULT 0,
  currentSessionStartedDate DATETIME,
  flagActive INTEGER NOT NULL DEFAULT 1,
  lastOverageReminderDate DATETIME,
  createdBy TEXT NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  editedBy TEXT,
  editedDate DATETIME,
  UNIQUE (issueKey)
);

CREATE TABLE IF NOT EXISTS tr_reminderlog (
  reminderLogID INTEGER PRIMARY KEY AUTOINCREMENT,
  issueKey TEXT NOT NULL,
  tier TEXT NOT NULL,
  sentDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tr_reminderlog_issuekey_tier
  ON tr_reminderlog (issueKey, tier);
