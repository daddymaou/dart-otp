CREATE TABLE admin_audit (
  id               bigserial PRIMARY KEY,
  actor_tenant_id  uuid NOT NULL,
  actor_email      text NOT NULL,
  action           text NOT NULL CHECK (action IN ('tenant_suspended', 'tenant_reactivated')),
  target_tenant_id uuid NOT NULL,
  reason           text NOT NULL CHECK (length(btrim(reason)) > 0),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX admin_audit_created_at_id_idx ON admin_audit (created_at DESC, id DESC);
