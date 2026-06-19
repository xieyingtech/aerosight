package store

import "time"

type User struct {
	ID        int32      `json:"id"`
	Name      string     `json:"name"`
	Email     *string    `json:"email"`
	Phone     *string    `json:"phone"`
	Password  *string    `json:"-"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type Team struct {
	ID        int32     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Project struct {
	ID              int32     `json:"id"`
	TeamID          int32     `json:"teamId"`
	Name            string    `json:"name"`
	Description     *string   `json:"description"`
	CreatedByUserID *int32    `json:"createdByUserId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	TeamName        *string   `json:"teamName,omitempty"`
	Role            *string   `json:"role,omitempty"`
	CreatedByName   *string   `json:"createdByUserName,omitempty"`
}

type Overview struct {
	Users    int64 `json:"users"`
	Teams    int64 `json:"teams"`
	Projects int64 `json:"projects"`
}

type TeamMembership struct {
	ID       int32     `json:"id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

type ManagedTeam struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type AdminTeam struct {
	ID          int32     `json:"id"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	MemberCount int64     `json:"memberCount"`
	OwnerUserID *int32    `json:"ownerUserId"`
	OwnerName   *string   `json:"ownerName"`
}

type DeviceItem struct {
	ID         int32      `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type AgentItem struct {
	ID          int32     `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type TaskItem struct {
	ID          int32     `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	TriggerType string    `json:"triggerType"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type IssueItem struct {
	ID        int32     `json:"id"`
	Number    int32     `json:"number"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  string    `json:"priority"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AssetItem struct {
	ID         int32      `json:"id"`
	Kind       string     `json:"kind"`
	MimeType   *string    `json:"mimeType"`
	CapturedAt *time.Time `json:"capturedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}
