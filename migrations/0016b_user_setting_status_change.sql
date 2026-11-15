-- One "status change" toggle replaces notifyEarlyFinish / notifyReturnedToTodo
-- (the old columns stay, unused). statusChangeStatuses = the destination
-- statuses that trigger a DM, comma separated; NULL/'' = every status.
ALTER TABLE ms_user_setting
  ADD COLUMN notifyStatusChange TINYINT(1) NOT NULL DEFAULT 1 AFTER notifyReturnedToTodo,
  ADD COLUMN statusChangeStatuses VARCHAR(500) NULL AFTER notifyStatusChange;
