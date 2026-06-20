package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Store) FindUserForLogin(ctx context.Context, email, phone *string) (User, error) {
	var user User
	query := `select id, name, email, phone, password, role, created_at, updated_at from users where `
	args := []any{}
	switch {
	case email != nil && phone != nil:
		query += `email = $1 or phone = $2`
		args = append(args, *email, *phone)
	case email != nil:
		query += `email = $1`
		args = append(args, *email)
	case phone != nil:
		query += `phone = $1`
		args = append(args, *phone)
	default:
		return User{}, pgx.ErrNoRows
	}
	err := s.pool.QueryRow(ctx, query, args...).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (s *Store) GetProfile(ctx context.Context, userID int32) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		select id, name, email, phone, null::text as password, role, created_at, updated_at
		from users
		where id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) UpdateProfile(ctx context.Context, userID int32, name, email, phone *string) ([]User, error) {
	_, err := s.pool.Exec(ctx, `
		update users
		set
			name = coalesce($2, name),
			email = case when $3::boolean then $4 else email end,
			phone = case when $5::boolean then $6 else phone end,
			updated_at = now()
		where id = $1
	`, userID, name, email != nil, emptyToNil(email), phone != nil, emptyToNil(phone))
	if err != nil {
		return nil, err
	}
	return s.GetProfile(ctx, userID)
}

func (s *Store) ListProfileTeams(ctx context.Context, userID int32) ([]TeamMembership, error) {
	rows, err := s.pool.Query(ctx, `
		select t.id, t.name, tm.role, tm.created_at
		from teams t
		inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
		order by t.name asc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TeamMembership{}
	for rows.Next() {
		var item TeamMembership
		if err := rows.Scan(&item.ID, &item.Name, &item.Role, &item.JoinedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListManagedTeams(ctx context.Context, userID int32) ([]ManagedTeam, error) {
	rows, err := s.pool.Query(ctx, `
		select t.id, t.name
		from teams t
		inner join team_members tm on tm.team_id = t.id and tm.user_id = $1 and tm.role in ('owner', 'admin')
		order by t.name asc
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ManagedTeam{}
	for rows.Next() {
		var item ManagedTeam
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListTeams(ctx context.Context, userID int32, search string) ([]TeamListItem, error) {
	query := `
		select t.id, t.name, current_members.role, count(all_members.id), t.created_at, t.updated_at
		from teams t
		inner join team_members current_members on current_members.team_id = t.id and current_members.user_id = $1
		left join team_members all_members on all_members.team_id = t.id
		where current_members.user_id = $1`
	args := []any{userID}
	if strings.TrimSpace(search) != "" {
		query += ` and t.name ilike $2`
		args = append(args, "%"+strings.TrimSpace(search)+"%")
	}
	query += `
		group by t.id, t.name, t.created_at, t.updated_at, current_members.role
		order by t.name asc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TeamListItem{}
	for rows.Next() {
		var item TeamListItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Role, &item.MemberCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetTeamDetail(ctx context.Context, userID, teamID int32) (TeamDetail, error) {
	var detail TeamDetail
	if err := s.pool.QueryRow(ctx, `
		select t.id, t.name, tm.role, count(all_members.id), t.created_at, t.updated_at
		from teams t
		inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
		left join team_members all_members on all_members.team_id = t.id
		where t.id = $2
		group by t.id, t.name, t.created_at, t.updated_at, tm.role
		limit 1
	`, userID, teamID).Scan(
		&detail.Team.ID,
		&detail.Team.Name,
		&detail.Team.Role,
		&detail.Team.MemberCount,
		&detail.Team.CreatedAt,
		&detail.Team.UpdatedAt,
	); err != nil {
		return TeamDetail{}, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, name, description, updated_at
		from projects
		where team_id = $1
		order by updated_at desc
	`, teamID)
	if err != nil {
		return TeamDetail{}, err
	}
	defer rows.Close()
	detail.Projects = []TeamProject{}
	for rows.Next() {
		var project TeamProject
		if err := rows.Scan(&project.ID, &project.Name, &project.Description, &project.UpdatedAt); err != nil {
			return TeamDetail{}, err
		}
		detail.Projects = append(detail.Projects, project)
	}
	return detail, rows.Err()
}

func (s *Store) ListProjects(ctx context.Context, userID int32, scope, search string) ([]Project, error) {
	query := `
		select p.id, p.team_id, p.name, p.description, p.created_by_user_id, p.created_at, p.updated_at, t.name, tm.role, null::text
		from projects p
		inner join teams t on p.team_id = t.id
		inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
		where tm.user_id = $1`
	args := []any{userID}
	arg := 2
	if scope == "joined" {
		query += ` and tm.role = 'member'`
	}
	if scope == "managed" {
		query += ` and tm.role in ('owner', 'admin')`
	}
	if strings.TrimSpace(search) != "" {
		query += ` and (p.name ilike $` + itoa(arg) + ` or p.description ilike $` + itoa(arg) + ` or t.name ilike $` + itoa(arg) + `)`
		args = append(args, "%"+strings.TrimSpace(search)+"%")
	}
	query += ` order by p.updated_at desc`
	return s.queryProjects(ctx, query, args...)
}

func (s *Store) GetProjectForUser(ctx context.Context, userID, projectID int32) (Project, error) {
	items, err := s.queryProjects(ctx, `
		select p.id, p.team_id, p.name, p.description, p.created_by_user_id, p.created_at, p.updated_at, t.name, tm.role, null::text
		from projects p
		inner join teams t on p.team_id = t.id
		inner join team_members tm on tm.team_id = t.id and tm.user_id = $1
		where p.id = $2
		limit 1
	`, userID, projectID)
	if err != nil {
		return Project{}, err
	}
	if len(items) == 0 {
		return Project{}, pgx.ErrNoRows
	}
	return items[0], nil
}

func (s *Store) UserCanManageTeam(ctx context.Context, userID, teamID int32) (bool, error) {
	var found int32
	err := s.pool.QueryRow(ctx, `
		select team_id from team_members
		where team_id = $1 and user_id = $2 and role in ('owner', 'admin')
		limit 1
	`, teamID, userID).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) CreateProject(ctx context.Context, teamID, userID int32, name string, description *string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		insert into projects (team_id, name, description, created_by_user_id)
		values ($1, $2, $3, $4)
		returning id, team_id, name, description, created_by_user_id, created_at, updated_at, null::text, null::text, null::text
	`, teamID, name, emptyToNil(description), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (s *Store) Overview(ctx context.Context) (Overview, error) {
	var out Overview
	err := s.pool.QueryRow(ctx, `
		select
			(select count(*) from users),
			(select count(*) from teams),
			(select count(*) from projects)
	`).Scan(&out.Users, &out.Teams, &out.Projects)
	return out, err
}

func (s *Store) ListAdminUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		select id, name, email, phone, null::text as password, role, created_at, updated_at
		from users
		order by created_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) CreateUser(ctx context.Context, name string, email, phone *string, role, password string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		insert into users (name, email, phone, role, password)
		values ($1, $2, $3, $4, $5)
		returning id, name, email, phone, null::text as password, role, created_at, updated_at
	`, name, emptyToNil(email), emptyToNil(phone), role, password)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) UpdateUser(ctx context.Context, id int32, name, email, phone, role, password *string) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		update users
		set
			name = coalesce($2, name),
			email = case when $3::boolean then $4 else email end,
			phone = case when $5::boolean then $6 else phone end,
			role = coalesce($7, role),
			password = coalesce($8, password),
			updated_at = now()
		where id = $1
		returning id, name, email, phone, null::text as password, role, created_at, updated_at
	`, id, name, email != nil, emptyToNil(email), phone != nil, emptyToNil(phone), role, password)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) DeleteUser(ctx context.Context, id int32) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		delete from users where id = $1
		returning id, name, email, phone, null::text as password, role, created_at, updated_at
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUsers(rows)
}

func (s *Store) ListAdminTeams(ctx context.Context) ([]AdminTeam, error) {
	rows, err := s.pool.Query(ctx, `
		select t.id, t.name, t.created_at, t.updated_at, count(tm.id),
			owner_users.id, owner_users.name
		from teams t
		left join team_members tm on tm.team_id = t.id
		left join team_members owners on owners.team_id = t.id and owners.role = 'owner'
		left join users owner_users on owner_users.id = owners.user_id
		group by t.id, t.name, t.created_at, t.updated_at, owner_users.id, owner_users.name
		order by t.name asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AdminTeam{}
	for rows.Next() {
		var item AdminTeam
		if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.UpdatedAt, &item.MemberCount, &item.OwnerUserID, &item.OwnerName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateTeam(ctx context.Context, name string, ownerUserID int32) ([]Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var team Team
	if err := tx.QueryRow(ctx, `
		insert into teams (name) values ($1)
		returning id, name, created_at, updated_at
	`, name).Scan(&team.ID, &team.Name, &team.CreatedAt, &team.UpdatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into team_members (team_id, user_id, role) values ($1, $2, 'owner')
	`, team.ID, ownerUserID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return []Team{team}, nil
}

func (s *Store) UserExists(ctx context.Context, id int32) (bool, error) {
	var found int32
	err := s.pool.QueryRow(ctx, `select id from users where id = $1`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) UpdateTeam(ctx context.Context, id int32, name *string, ownerUserID *int32) ([]Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if name != nil {
		if _, err := tx.Exec(ctx, `update teams set name = $2, updated_at = now() where id = $1`, id, *name); err != nil {
			return nil, err
		}
	}
	if ownerUserID != nil {
		if _, err := tx.Exec(ctx, `
			update team_members set role = 'admin'
			where team_id = $1 and role = 'owner' and user_id <> $2
		`, id, *ownerUserID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			insert into team_members (team_id, user_id, role)
			values ($1, $2, 'owner')
			on conflict (team_id, user_id) do update set role = 'owner'
		`, id, *ownerUserID); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `select id, name, created_at, updated_at from teams where id = $1`, id)
	if err != nil {
		return nil, err
	}
	items, err := scanTeams(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) DeleteTeam(ctx context.Context, id int32) ([]Team, error) {
	rows, err := s.pool.Query(ctx, `delete from teams where id = $1 returning id, name, created_at, updated_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTeams(rows)
}

func (s *Store) ListAdminProjects(ctx context.Context) ([]Project, error) {
	return s.queryProjects(ctx, `
		select p.id, p.team_id, p.name, p.description, p.created_by_user_id, p.created_at, p.updated_at, t.name, null::text, u.name
		from projects p
		inner join teams t on t.id = p.team_id
		left join users u on u.id = p.created_by_user_id
		order by p.created_at desc
	`)
}

func (s *Store) UpdateProject(ctx context.Context, id int32, teamID *int32, name, description *string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		update projects
		set
			team_id = coalesce($2, team_id),
			name = coalesce($3, name),
			description = case when $4::boolean then $5 else description end,
			updated_at = now()
		where id = $1
		returning id, team_id, name, description, created_by_user_id, created_at, updated_at, null::text, null::text, null::text
	`, id, teamID, name, description != nil, emptyToNil(description))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (s *Store) DeleteProject(ctx context.Context, id int32) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		delete from projects where id = $1
		returning id, team_id, name, description, created_by_user_id, created_at, updated_at, null::text, null::text, null::text
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (s *Store) ListDevices(ctx context.Context, projectID int32) ([]DeviceItem, error) {
	rows, err := s.pool.Query(ctx, `select id, name, type, status, last_seen_at, updated_at from devices where project_id = $1 order by updated_at desc`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DeviceItem{}
	for rows.Next() {
		var item DeviceItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Status, &item.LastSeenAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListAgents(ctx context.Context, projectID int32) ([]AgentItem, error) {
	rows, err := s.pool.Query(ctx, `select id, name, description, status, updated_at from agents where project_id = $1 order by updated_at desc`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AgentItem{}
	for rows.Next() {
		var item AgentItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Status, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListTasks(ctx context.Context, projectID int32) ([]TaskItem, error) {
	rows, err := s.pool.Query(ctx, `select id, name, description, trigger_type, status, updated_at from tasks where project_id = $1 order by updated_at desc`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TaskItem{}
	for rows.Next() {
		var item TaskItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.TriggerType, &item.Status, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListIssues(ctx context.Context, projectID int32) ([]IssueItem, error) {
	rows, err := s.pool.Query(ctx, `select id, number, title, status, priority, updated_at from issues where project_id = $1 order by updated_at desc`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []IssueItem{}
	for rows.Next() {
		var item IssueItem
		if err := rows.Scan(&item.ID, &item.Number, &item.Title, &item.Status, &item.Priority, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListAssets(ctx context.Context, projectID int32) ([]AssetItem, error) {
	rows, err := s.pool.Query(ctx, `select id, kind, mime_type, captured_at, created_at from assets where project_id = $1 order by created_at desc`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AssetItem{}
	for rows.Next() {
		var item AssetItem
		if err := rows.Scan(&item.ID, &item.Kind, &item.MimeType, &item.CapturedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) queryProjects(ctx context.Context, query string, args ...any) ([]Project, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func scanUsers(rows pgx.Rows) ([]User, error) {
	items := []User{}
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Phone, &item.Password, &item.Role, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanTeams(rows pgx.Rows) ([]Team, error) {
	items := []Team{}
	for rows.Next() {
		var item Team
		if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanProjects(rows pgx.Rows) ([]Project, error) {
	items := []Project{}
	for rows.Next() {
		var item Project
		if err := rows.Scan(&item.ID, &item.TeamID, &item.Name, &item.Description, &item.CreatedByUserID, &item.CreatedAt, &item.UpdatedAt, &item.TeamName, &item.Role, &item.CreatedByName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func emptyToNil(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func itoa(value int) string {
	return string(rune('0' + value))
}
