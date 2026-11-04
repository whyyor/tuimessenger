-- v2 -> v3: Client API: hidden events, logins, settings, sent transactions
ALTER TABLE local_events ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
UPDATE local_events SET hidden = 1
WHERE json_extract(CAST(content AS TEXT), '$."com.beeper.exclude_from_timeline"') = 1;

CREATE TABLE access_tokens (
	token_hash TEXT    PRIMARY KEY,
	device_id  TEXT    NOT NULL,
	created_at INTEGER NOT NULL
) WITHOUT ROWID;

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE sent_transactions (
	txn_id   TEXT PRIMARY KEY,
	event_id TEXT NOT NULL
) WITHOUT ROWID;
