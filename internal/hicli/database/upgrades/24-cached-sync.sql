-- v24 (compatible with v10+): Add columns needed for caching room list on the frontend
ALTER TABLE room ADD COLUMN mod_timestamp INTEGER NOT NULL DEFAULT (unixepoch('subsec')*1000);
ALTER TABLE account_data ADD COLUMN mod_timestamp INTEGER NOT NULL DEFAULT (unixepoch('subsec')*1000);
ALTER TABLE room_account_data ADD COLUMN mod_timestamp INTEGER NOT NULL DEFAULT (unixepoch('subsec')*1000);
CREATE INDEX room_mod_timestamp_idx ON room (mod_timestamp);
CREATE INDEX account_data_mod_timestamp_idx ON account_data (mod_timestamp);
CREATE INDEX room_account_data_mod_timestamp_idx ON account_data (mod_timestamp);
