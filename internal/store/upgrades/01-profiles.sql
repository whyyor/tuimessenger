-- v1 -> v2: Store user profiles
CREATE TABLE local_profiles (
	user_id     TEXT PRIMARY KEY,
	displayname TEXT NOT NULL DEFAULT '',
	avatar_url  TEXT NOT NULL DEFAULT ''
) WITHOUT ROWID;
