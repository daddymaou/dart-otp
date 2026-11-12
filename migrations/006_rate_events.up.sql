CREATE TABLE rate_events (
  id         bigserial PRIMARY KEY,
  bucket     text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON rate_events (bucket, created_at);
