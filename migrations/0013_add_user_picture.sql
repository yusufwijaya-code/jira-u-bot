-- Google profile picture of each user, refreshed from the Sign-In ID token's
-- "picture" claim on every login (Tech Spec "Admin Panel Revamp" — user
-- avatars). Kept out of ms_user so the registration repository is untouched.
CREATE TABLE IF NOT EXISTS ms_user_picture (
  userID      BIGINT        NOT NULL,
  pictureURL  VARCHAR(1000) NOT NULL DEFAULT '',
  updatedDate DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (userID),
  CONSTRAINT fk_ms_user_picture_user FOREIGN KEY (userID) REFERENCES ms_user (userID) ON DELETE CASCADE
);
