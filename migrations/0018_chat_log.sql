-- Riwayat pesan: every message Nudge sends and every Chat event/command a
-- user sends to it, for the admin "Riwayat pesan" page.
CREATE TABLE IF NOT EXISTS tr_chatlog (
  chatLogID   BIGINT AUTO_INCREMENT PRIMARY KEY,
  email       VARCHAR(255)  NOT NULL,
  direction   VARCHAR(3)    NOT NULL COMMENT 'in = from the user, out = from Nudge',
  kind        VARCHAR(20)   NOT NULL COMMENT 'reminder, message, reply, command, click, added, removed',
  channel     VARCHAR(10)   NOT NULL DEFAULT '' COMMENT 'app, operator, fallback',
  status      VARCHAR(10)   NOT NULL COMMENT 'received, sent, failed, skipped',
  body        TEXT          NOT NULL,
  errorText   VARCHAR(500)  NOT NULL DEFAULT '',
  createdDate DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_chatlog_email_date (email, createdDate),
  KEY idx_chatlog_date (createdDate)
);
