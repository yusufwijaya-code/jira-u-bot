-- OMS & CORE exactly as the old hardcoded rules. INSERT IGNORE: never
-- overwrites an admin's edits on later starts (AC1.3).
INSERT IGNORE INTO ms_tracking_rule (projectKey, projectName, trackedStatuses, doneStatus, reviewStatus, storyPointFields, storyPointWriteField, storyPointWriteFieldSubtask) VALUES
  ('OMS',  'ESB OMS',  JSON_ARRAY('Code Process'),                'Code Review',    NULL,          JSON_ARRAY('customfield_10016'),                      'customfield_10016', 'customfield_10016'),
  ('CORE', 'ESB Core', JSON_ARRAY('Code Process', 'CODE REVIEW'), 'Code Completed', 'CODE REVIEW', JSON_ARRAY('customfield_10125', 'customfield_11053'), 'customfield_10125', 'customfield_11053');
