create table approval_requests (
  id uuid primary key,
  project_id integer not null,
  team_id integer not null,
  resource_type text not null,
  resource_id text not null,
  action text not null,
  requested_by_user_id integer not null,
  status text not null default 'pending',
  required_approvals integer not null default 1,
  require_separation boolean not null default true,
  expires_at timestamptz not null,
  context_json jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  decided_at timestamptz,
  constraint approval_requests_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint approval_requests_requester_member_fk
    foreign key (team_id, requested_by_user_id) references team_members(team_id, user_id) on delete restrict,
  constraint approval_requests_status_valid check (status in ('pending', 'approved', 'rejected', 'expired')),
  constraint approval_requests_required_valid check (required_approvals > 0),
  constraint approval_requests_id_project_unique unique (id, project_id)
);
--> statement-breakpoint
create table approvals (
  id bigserial primary key,
  project_id integer not null,
  team_id integer not null,
  approval_request_id uuid not null,
  approver_user_id integer not null,
  decision text not null,
  reason text not null,
  decided_at timestamptz not null default now(),
  constraint approvals_project_team_fk
    foreign key (project_id, team_id) references projects(id, team_id) on delete cascade,
  constraint approvals_request_project_fk
    foreign key (approval_request_id, project_id) references approval_requests(id, project_id) on delete cascade,
  constraint approvals_approver_member_fk
    foreign key (team_id, approver_user_id) references team_members(team_id, user_id) on delete restrict,
  constraint approvals_decision_valid check (decision in ('approved', 'rejected')),
  constraint approvals_request_approver_unique unique (approval_request_id, approver_user_id)
);
--> statement-breakpoint
create index approval_requests_project_status_idx on approval_requests(project_id, status, expires_at);
--> statement-breakpoint
create index approvals_request_decided_idx on approvals(approval_request_id, decided_at);
--> statement-breakpoint
create or replace function validate_approval_decision()
returns trigger language plpgsql as $$
declare request approval_requests%rowtype;
begin
  select * into request from approval_requests where id = new.approval_request_id for update;
  if request.status <> 'pending' then
    raise exception 'approval request is not pending' using errcode = '55000';
  end if;
  if request.expires_at <= new.decided_at then
    raise exception 'approval request expired' using errcode = '55000';
  end if;
  if request.require_separation and request.requested_by_user_id = new.approver_user_id then
    raise exception 'requester cannot approve own request' using errcode = '42501';
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger approvals_validate_decision
before insert on approvals for each row execute function validate_approval_decision();
--> statement-breakpoint
create or replace function project_approval_request_status()
returns trigger language plpgsql as $$
declare required_count integer;
declare approved_count integer;
begin
  if new.decision = 'rejected' then
    update approval_requests set status = 'rejected', decided_at = new.decided_at
      where id = new.approval_request_id;
    return new;
  end if;
  select required_approvals into required_count from approval_requests where id = new.approval_request_id;
  select count(*) into approved_count from approvals
    where approval_request_id = new.approval_request_id and decision = 'approved';
  if approved_count >= required_count then
    update approval_requests set status = 'approved', decided_at = new.decided_at
      where id = new.approval_request_id;
  end if;
  return new;
end;
$$;
--> statement-breakpoint
create trigger approvals_project_request_status
after insert on approvals for each row execute function project_approval_request_status();
