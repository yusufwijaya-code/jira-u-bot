CREATE TABLE IF NOT EXISTS chat_command_poll_cursor (
  googleChatDMSpaceID VARCHAR(255) NOT NULL,
  lastProcessedMessageCreateTime DATETIME NOT NULL,
  PRIMARY KEY (googleChatDMSpaceID)
);
