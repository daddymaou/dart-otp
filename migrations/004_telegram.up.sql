CREATE TABLE link_tokens (
  token_hash text PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  phone      text NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE telegram_links (
  phone            text PRIMARY KEY,
  chat_id          bigint NOT NULL,
  telegram_user_id bigint NOT NULL,
  status           text NOT NULL DEFAULT 'active',
  linked_at        timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON telegram_links (chat_id);
