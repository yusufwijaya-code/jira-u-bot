-- Aturan Board (Tech Spec "Aturan Board (Admin CRUD)"): which statuses of a
-- Jira project count toward a ticket's budget and where its Story Points live.
CREATE TABLE IF NOT EXISTS ms_tracking_rule (
  projectKey                  VARCHAR(20)  NOT NULL                COMMENT 'Jira project key, UPPER',
  projectName                 VARCHAR(255) NOT NULL DEFAULT ''     COMMENT 'Project name cached from Jira',
  trackedStatuses             JSON         NOT NULL                COMMENT 'Statuses the budget clock runs in',
  doneStatus                  VARCHAR(100) NOT NULL                COMMENT 'Finished status (early completion)',
  reviewStatus                VARCHAR(100) NULL                    COMMENT 'Tracked review status; must be in trackedStatuses',
  storyPointFields            JSON         NOT NULL                COMMENT 'customfield ids, order = read priority',
  storyPointWriteField        VARCHAR(50)  NOT NULL                COMMENT 'SP field Buat Card writes for a normal card',
  storyPointWriteFieldSubtask VARCHAR(50)  NOT NULL                COMMENT 'SP field Buat Card writes for a subtask',
  isActive                    TINYINT(1)   NOT NULL DEFAULT 1,
  editedBy                    VARCHAR(255) NOT NULL DEFAULT 'seed' COMMENT 'Last admin email',
  editedDate                  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (projectKey)
);
