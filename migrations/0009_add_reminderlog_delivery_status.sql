ALTER TABLE tr_reminderlog ADD COLUMN deliveryStatus VARCHAR(10) NOT NULL DEFAULT 'sent' AFTER tier;
