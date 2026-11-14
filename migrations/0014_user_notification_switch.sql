-- Tech Spec "Notification On-Off Consent": master switch for automatic
-- notifications. Existing users keep receiving them (DEFAULT 1); new users
-- are inserted with 0 by registration. notificationChoiceDate NULL = the
-- user hasn't picked on/off yet (the dashboard asks after login).
ALTER TABLE ms_user
  ADD COLUMN notificationsEnabled TINYINT(1) NOT NULL DEFAULT 1,
  ADD COLUMN notificationChoiceDate DATETIME NULL;
