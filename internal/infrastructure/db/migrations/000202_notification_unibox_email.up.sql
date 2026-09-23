-- The unibox message an inbound notification is about, so the notification
-- goes with the message (000204) and is read when the message is read.
ALTER TABLE notifications ADD COLUMN unibox_email_id UUID;
