CREATE TABLE IF NOT EXISTS ms_user_setting (
  userID BIGINT NOT NULL,
  reminderTiers VARCHAR(200) NOT NULL DEFAULT '2h,1h,30m,15m,5m,1m',
  overageTiers VARCHAR(200) NOT NULL DEFAULT '',
  overageRepeatMinutes SMALLINT NULL DEFAULT 5,
  notifyEarlyFinish TINYINT(1) NOT NULL DEFAULT 1,
  notifyReturnedToTodo TINYINT(1) NOT NULL DEFAULT 1,
  mutedUntil DATETIME NULL,
  quietHoursStart TIME NULL,
  quietHoursEnd TIME NULL,
  updatedDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (userID),
  CONSTRAINT fk_ms_user_setting_user FOREIGN KEY (userID) REFERENCES ms_user (userID) ON DELETE CASCADE
);

-- Carry the legacy per-user tier subset (0006) over. INSERT IGNORE keeps
-- this idempotent: once a user has a ms_user_setting row it is never
-- overwritten by a re-run.
INSERT IGNORE INTO ms_user_setting (userID, reminderTiers)
SELECT userID, reminderTierPreference
FROM ms_user
WHERE reminderTierPreference IS NOT NULL;
