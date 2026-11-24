ALTER TABLE admin_audit
  DROP CONSTRAINT admin_audit_action_check,
  ADD CONSTRAINT admin_audit_action_check
    CHECK (action IN ('tenant_suspended', 'tenant_reactivated', 'tenant_plan_changed')),
  ADD COLUMN previous_plan text,
  ADD COLUMN new_plan text,
  ADD CONSTRAINT admin_audit_plan_pair_check
    CHECK ((action = 'tenant_plan_changed') = (previous_plan IS NOT NULL AND new_plan IS NOT NULL));
