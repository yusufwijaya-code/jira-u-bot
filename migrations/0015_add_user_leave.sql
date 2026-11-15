-- Tech Spec "Papan Tiket & Cuti": a user's full leave days. A leave day is a
-- non-working day for that user only (no tracked time, no reminders).
CREATE TABLE IF NOT EXISTS tr_user_leave (
  userLeaveID BIGINT AUTO_INCREMENT PRIMARY KEY,
  email       VARCHAR(255) NOT NULL,
  leaveDate   DATE         NOT NULL,
  note        VARCHAR(200) NULL,
  createdDate DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_user_leave_email_date (email, leaveDate),
  KEY idx_user_leave_date (leaveDate)
);
