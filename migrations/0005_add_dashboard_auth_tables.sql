CREATE TABLE IF NOT EXISTS tr_usersession (
  sessionID VARCHAR(64) NOT NULL,
  userID BIGINT NOT NULL,
  expiresAt DATETIME NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (sessionID),
  INDEX idx_tr_usersession_userid (userID),
  CONSTRAINT fk_tr_usersession_user FOREIGN KEY (userID) REFERENCES ms_user(userID) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tr_usercredential (
  userCredentialID BIGINT NOT NULL AUTO_INCREMENT,
  userID BIGINT NOT NULL,
  jiraEmail VARCHAR(255) NOT NULL,
  jiraAPITokenEncrypted VARBINARY(1024) NOT NULL,
  flagActive TINYINT(1) NOT NULL DEFAULT 1,
  createdBy VARCHAR(255) NOT NULL,
  createdDate DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  editedBy VARCHAR(255),
  editedDate DATETIME,
  PRIMARY KEY (userCredentialID),
  UNIQUE KEY uq_tr_usercredential_userid (userID),
  CONSTRAINT fk_tr_usercredential_user FOREIGN KEY (userID) REFERENCES ms_user(userID) ON DELETE CASCADE
);
