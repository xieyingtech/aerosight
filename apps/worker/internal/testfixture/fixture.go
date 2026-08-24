package testfixture

type Fixture struct {
	Projects []Project `json:"projects"`
}

type Project struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	TeamName string   `json:"teamName"`
	Owner    Owner    `json:"owner"`
	Devices  []Device `json:"devices"`
}

type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Device struct {
	ExternalID   string   `json:"externalId"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Capabilities []string `json:"capabilities"`
}
