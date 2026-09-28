ALTER TABLE agent_write_approvals DROP CONSTRAINT agent_write_approvals_status_check;
ALTER TABLE agent_write_approvals ADD CONSTRAINT agent_write_approvals_status_check
 CHECK (status IN ('pending','executing','succeeded','failed','rejected'));
