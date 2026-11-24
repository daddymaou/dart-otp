DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM admin_audit WHERE action = 'tenant_plan_changed') THEN
    RAISE EXCEPTION 'Cannot remove admin plan-change audit data; archive these entries before rolling back';
  END IF;
END $$;

ALTER TABLE admin_audit
  DROP CONSTRAINT admin_audit_plan_pair_check,
  DROP CONSTRAINT admin_audit_action_check,
  ADD CONSTRAINT admin_audit_action_check
    CHECK (action IN ('tenant_suspended', 'tenant_reactivated')),
  DROP COLUMN previous_plan,
  DROP COLUMN new_plan;
