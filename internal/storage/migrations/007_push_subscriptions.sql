-- Web Push subscriptions (2026-11-15): one row per browser that enabled
-- notifications, tied to the operator who was signed in. prefs_json holds the
-- per-category switches ({"watering":true,"dry":true,"reservoir":true,"faults":false}).
CREATE TABLE IF NOT EXISTS push_subscriptions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  username    TEXT NOT NULL,
  endpoint    TEXT NOT NULL UNIQUE,
  p256dh      TEXT NOT NULL,
  auth        TEXT NOT NULL,
  prefs_json  TEXT NOT NULL DEFAULT '{}',
  label       TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_ok_at  TEXT,
  failures    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user ON push_subscriptions(username);
