CREATE TABLE otps (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  phone           text NOT NULL,
  code_hash       text NOT NULL,
  salt            text NOT NULL,
  template_id     uuid REFERENCES templates(id) ON DELETE SET NULL,
  status          text NOT NULL DEFAULT 'pending',
  delivery_status text NOT NULL DEFAULT 'queued',
  delivery_error  text,
  attempts        int  NOT NULL DEFAULT 0,
  max_attempts    int  NOT NULL,
  is_test         boolean NOT NULL DEFAULT false,
  expires_at      timestamptz NOT NULL,
  verified_at     timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON otps (tenant_id, phone, created_at DESC);
CREATE INDEX ON otps (tenant_id, created_at DESC);
