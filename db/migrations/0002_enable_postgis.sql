do $$
begin
  if not exists (
    select 1 from pg_available_extensions where name = 'postgis'
  ) then
    raise exception using
      message = 'PostGIS extension is required but is not available on this PostgreSQL server',
      hint = 'Install the PostGIS package for the server version, then rerun the migration';
  end if;
end
$$;
--> statement-breakpoint
create extension if not exists postgis;
