CREATE TABLE agent_write_approvals (
  id uuid PRIMARY KEY,
  project_id integer NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  session_id integer NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
  user_id integer NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tool_name text NOT NULL,
  arguments jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'rejected')),
  result jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL DEFAULT (now() + interval '30 minutes'),
  decided_at timestamptz
);
CREATE INDEX agent_write_approvals_session_idx ON agent_write_approvals(project_id, session_id, user_id, created_at DESC);
