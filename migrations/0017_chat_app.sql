-- Tech Spec "Nudge sebagai Google Chat App": the user's DM space with the
-- Nudge Chat App (NULL = not activated), the Chat user id from the event,
-- when it was activated, and the chosen sender ('nudge' | 'operator';
-- NULL = automatic: nudge when activated, operator otherwise).
ALTER TABLE ms_user
  ADD COLUMN chatAppSpaceID VARCHAR(255) NULL,
  ADD COLUMN chatAppUserID VARCHAR(64) NULL,
  ADD COLUMN chatAppActivatedDate DATETIME NULL,
  ADD COLUMN chatSender VARCHAR(16) NULL,
  ADD INDEX idx_ms_user_chatAppSpaceID (chatAppSpaceID);
