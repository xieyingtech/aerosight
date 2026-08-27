create table generated_reports (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  source_type text not null,
  source_id text not null,
  title text not null,
  current_published_version_id uuid,
  created_by_user_id integer,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint generated_reports_source_type_valid check(source_type in('task_run','perception_event')),
  constraint generated_reports_project_source_unique unique(project_id,source_type,source_id),
  constraint generated_reports_id_project_unique unique(id,project_id),
  constraint generated_reports_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

create table generated_report_versions (
  id uuid primary key default gen_random_uuid(),
  project_id integer not null,
  team_id integer not null,
  generated_report_id uuid not null,
  version integer not null,
  status text not null default 'draft',
  completeness text not null,
  content_json jsonb not null default '{}'::jsonb,
  data_gaps_json jsonb not null default '[]'::jsonb,
  created_by_user_id integer,
  published_by_user_id integer,
  created_at timestamptz not null default now(),
  published_at timestamptz,
  constraint generated_report_versions_status_valid check(status in('draft','published','retired')),
  constraint generated_report_versions_completeness_valid check(completeness in('complete','incomplete','failed')),
  constraint generated_report_versions_version_positive check(version>0),
  constraint generated_report_versions_report_version_unique unique(generated_report_id,version),
  constraint generated_report_versions_id_project_unique unique(id,project_id),
  constraint generated_report_versions_report_project_fk foreign key(generated_report_id,project_id) references generated_reports(id,project_id) on delete cascade,
  constraint generated_report_versions_project_team_fk foreign key(project_id,team_id) references projects(id,team_id) on delete cascade
);

alter table generated_reports add constraint generated_reports_current_version_project_fk
  foreign key(current_published_version_id,project_id) references generated_report_versions(id,project_id) on delete set null;

create table generated_report_evidence (
  id bigserial primary key,
  project_id integer not null,
  report_version_id uuid not null,
  evidence_type text not null,
  evidence_id text not null,
  evidence_version text not null,
  asset_id integer,
  checksum_sha256 text,
  href text not null,
  created_at timestamptz not null default now(),
  constraint generated_report_evidence_type_valid check(evidence_type in('task_run','task_version','device','track','step','event','feedback','asset')),
  constraint generated_report_evidence_version_unique unique(report_version_id,evidence_type,evidence_id,evidence_version),
  constraint generated_report_evidence_report_project_fk foreign key(report_version_id,project_id) references generated_report_versions(id,project_id) on delete cascade,
  constraint generated_report_evidence_asset_project_fk foreign key(asset_id,project_id) references assets(id,project_id) on delete restrict
);

create unique index generated_report_versions_one_draft_idx on generated_report_versions(generated_report_id) where status='draft';
create index generated_reports_project_updated_idx on generated_reports(project_id,updated_at desc);
create index generated_report_evidence_asset_idx on generated_report_evidence(project_id,asset_id) where asset_id is not null;
