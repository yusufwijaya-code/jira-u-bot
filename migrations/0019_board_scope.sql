-- The board the user picked in the web navbar (NULL = "Semua board").
-- Chat commands (/n-mytickets, /n-dashboard, /n-mute) follow it.
ALTER TABLE ms_user
  ADD COLUMN selectedBoardID BIGINT NULL;
