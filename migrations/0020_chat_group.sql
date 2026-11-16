-- Auto Join Google Group: when the user's email was added to the group that
-- makes the Nudge Chat App visible, and the last error doing so.
ALTER TABLE ms_user
  ADD COLUMN chatGroupAddedDate DATETIME NULL,
  ADD COLUMN chatGroupError VARCHAR(255) NULL;
