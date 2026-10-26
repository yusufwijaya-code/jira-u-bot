CREATE TABLE IF NOT EXISTS ms_user (
  userID BIGINT NOT NULL AUTO_INCREMENT,
  userName VARCHAR(255) NOT NULL,
  email VARCHAR(255) NOT NULL,
  flagActive TINYINT(1) NOT NULL DEFAULT 1,
  createdBy VARCHAR(255) NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  editedBy VARCHAR(255),
  editedDate DATETIME,
  googleChatDMSpaceID VARCHAR(255),
  PRIMARY KEY (userID),
  UNIQUE KEY uq_ms_user_email (email)
);

CREATE TABLE IF NOT EXISTS tr_issuesession (
  issueSessionID BIGINT NOT NULL AUTO_INCREMENT,
  issueKey VARCHAR(50) NOT NULL,
  projectKey VARCHAR(20) NOT NULL,
  storyPoints DOUBLE NOT NULL,
  accumulatedSeconds BIGINT NOT NULL DEFAULT 0,
  currentSessionStartedDate DATETIME,
  flagActive TINYINT(1) NOT NULL DEFAULT 1,
  lastOverageReminderDate DATETIME,
  assigneeEmail VARCHAR(255),
  issueSummary VARCHAR(500),
  createdBy VARCHAR(255) NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  editedBy VARCHAR(255),
  editedDate DATETIME,
  PRIMARY KEY (issueSessionID),
  UNIQUE KEY uq_tr_issuesession_issuekey (issueKey)
);

CREATE TABLE IF NOT EXISTS tr_reminderlog (
  reminderLogID BIGINT NOT NULL AUTO_INCREMENT,
  issueKey VARCHAR(50) NOT NULL,
  tier VARCHAR(20) NOT NULL,
  sentDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (reminderLogID),
  INDEX idx_tr_reminderlog_issuekey_tier (issueKey, tier)
);
