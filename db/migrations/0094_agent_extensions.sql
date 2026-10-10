CREATE TABLE agent_skills (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 slug text NOT NULL UNIQUE,
 name text NOT NULL,
 description text NOT NULL DEFAULT '',
 body text NOT NULL,
 enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1,
 created_by_user_id integer NOT NULL REFERENCES users(id),
 updated_by_user_id integer NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE agent_mcp_servers (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name text NOT NULL,
 endpoint text NOT NULL,
 credential_envelope_json jsonb,
 enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1,
 tools_json jsonb NOT NULL DEFAULT '[]',
 created_by_user_id integer NOT NULL REFERENCES users(id),
 updated_by_user_id integer NOT NULL REFERENCES users(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
