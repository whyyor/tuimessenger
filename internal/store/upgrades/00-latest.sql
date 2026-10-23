-- v0 -> v1: Initial schema
CREATE TABLE local_events (
	pos             INTEGER PRIMARY KEY,
	event_id        TEXT    NOT NULL UNIQUE,
	room_id         TEXT    NOT NULL,
	sender          TEXT    NOT NULL,
	type            TEXT    NOT NULL,
	state_key       TEXT,
	content         BLOB    NOT NULL,
	redacts         TEXT,
	unsigned        BLOB,
	stream_order    INTEGER NOT NULL,
	stream_suborder INTEGER NOT NULL DEFAULT 0,
	timestamp       INTEGER NOT NULL
);
CREATE INDEX local_events_room_order_idx ON local_events (room_id, stream_order, stream_suborder);

CREATE TABLE local_current_state (
	room_id   TEXT NOT NULL,
	type      TEXT NOT NULL,
	state_key TEXT NOT NULL,
	event_id  TEXT NOT NULL REFERENCES local_events (event_id),
	PRIMARY KEY (room_id, type, state_key)
) WITHOUT ROWID;

CREATE TABLE local_receipts (
	room_id   TEXT    NOT NULL,
	user_id   TEXT    NOT NULL,
	type      TEXT    NOT NULL,
	event_id  TEXT    NOT NULL,
	timestamp INTEGER NOT NULL,
	pos       INTEGER NOT NULL,
	PRIMARY KEY (room_id, user_id, type)
) WITHOUT ROWID;
CREATE INDEX local_receipts_pos_idx ON local_receipts (pos);

CREATE TABLE local_account_data (
	room_id TEXT    NOT NULL,
	type    TEXT    NOT NULL,
	content BLOB    NOT NULL,
	pos     INTEGER NOT NULL,
	PRIMARY KEY (room_id, type)
) WITHOUT ROWID;
CREATE INDEX local_account_data_pos_idx ON local_account_data (pos);
