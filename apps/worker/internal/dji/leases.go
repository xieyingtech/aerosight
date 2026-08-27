package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type AdapterLease struct {
	AdapterID  int64
	ProjectID  int
	Epoch      int64
	BrokerURL  string
	SecretRef  string
	ConfigJSON json.RawMessage
}

type LeaseRepository interface {
	Claim(context.Context, string, int, time.Duration) ([]AdapterLease, error)
	Renew(context.Context, AdapterLease, string, time.Duration) (bool, error)
	Release(context.Context, AdapterLease, string) error
	UpdateStatus(context.Context, AdapterLease, string, string, string) error
}

type SQLLeaseRepository struct{ db *sql.DB }

func NewSQLLeaseRepository(db *sql.DB) *SQLLeaseRepository { return &SQLLeaseRepository{db: db} }

func (repository *SQLLeaseRepository) Claim(
	ctx context.Context, owner string, limit int, lease time.Duration,
) ([]AdapterLease, error) {
	rows, err := repository.db.QueryContext(ctx, `
		with candidates as (
			select adapter.id, adapter.project_id, profile.mqtt_endpoint,
			       coalesce(adapter.secret_ref, profile.secret_ref) as secret_ref,
			       adapter.config_json
			from device_adapters adapter
			join device_network_profiles profile
			  on profile.id = adapter.network_profile_id and profile.project_id = adapter.project_id
			where adapter.adapter_type = 'dji'
			  and adapter.status in ('connecting', 'connected', 'degraded')
			  and profile.status = 'valid'
			  and (adapter.lease_expires_at is null or adapter.lease_expires_at < now())
			order by adapter.id
			for update of adapter skip locked
			limit $1
		)
		update device_adapters adapter
		set lease_owner = $2, lease_expires_at = now() + ($3 * interval '1 millisecond'),
		    connection_epoch = adapter.connection_epoch + 1, status = 'connecting', updated_at = now()
		from candidates
		where adapter.id = candidates.id
		returning adapter.id, adapter.project_id, adapter.connection_epoch,
		          candidates.mqtt_endpoint, candidates.secret_ref, candidates.config_json`,
		limit, owner, lease.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leases []AdapterLease
	for rows.Next() {
		var lease AdapterLease
		if err := rows.Scan(&lease.AdapterID, &lease.ProjectID, &lease.Epoch, &lease.BrokerURL, &lease.SecretRef, &lease.ConfigJSON); err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

func (repository *SQLLeaseRepository) Renew(
	ctx context.Context, lease AdapterLease, owner string, duration time.Duration,
) (bool, error) {
	result, err := repository.db.ExecContext(ctx, `
		update device_adapters
		set lease_expires_at = now() + ($5 * interval '1 millisecond'), updated_at = now()
		where id = $1 and project_id = $2 and connection_epoch = $3 and lease_owner = $4
		  and lease_expires_at >= now()`,
		lease.AdapterID, lease.ProjectID, lease.Epoch, owner, duration.Milliseconds())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (repository *SQLLeaseRepository) Release(ctx context.Context, lease AdapterLease, owner string) error {
	_, err := repository.db.ExecContext(ctx, `
		update device_adapters set lease_owner = null, lease_expires_at = null, updated_at = now()
		where id = $1 and project_id = $2 and connection_epoch = $3 and lease_owner = $4`,
		lease.AdapterID, lease.ProjectID, lease.Epoch, owner)
	return err
}

func (repository *SQLLeaseRepository) UpdateStatus(
	ctx context.Context, lease AdapterLease, owner, status, code string,
) error {
	_, err := repository.db.ExecContext(ctx, `
		update device_adapters
		set status = $5,
		    last_health_json = jsonb_build_object('ok', $5 = 'connected', 'code', $6::text, 'connectionEpoch', $3),
		    last_checked_at = now(),
		    last_connected_at = case when $5 = 'connected' then now() else last_connected_at end,
		    updated_at = now()
		where id = $1 and project_id = $2 and connection_epoch = $3 and lease_owner = $4`,
		lease.AdapterID, lease.ProjectID, lease.Epoch, owner, status, code)
	return err
}

type MQTTCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SecretResolver interface {
	ResolveMQTT(context.Context, string) (MQTTCredentials, error)
}

type EnvironmentSecretResolver struct{}

func (EnvironmentSecretResolver) ResolveMQTT(_ context.Context, reference string) (MQTTCredentials, error) {
	if !strings.HasPrefix(reference, "env://") {
		return MQTTCredentials{}, errors.New("DJI_SECRET_PROVIDER_UNSUPPORTED")
	}
	name := strings.TrimPrefix(reference, "env://")
	if name == "" || strings.ContainsAny(name, "/?#") {
		return MQTTCredentials{}, errors.New("DJI_SECRET_REFERENCE_INVALID")
	}
	raw, exists := os.LookupEnv(name)
	if !exists {
		return MQTTCredentials{}, errors.New("DJI_SECRET_NOT_FOUND")
	}
	var credentials MQTTCredentials
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
		return MQTTCredentials{}, errors.New("DJI_SECRET_FORMAT_INVALID")
	}
	if strings.TrimSpace(credentials.Username) == "" || credentials.Password == "" {
		return MQTTCredentials{}, errors.New("DJI_SECRET_FORMAT_INVALID")
	}
	return credentials, nil
}

type adapterSessionConfig struct {
	ClientID       string   `json:"clientId"`
	Topics         []string `json:"topics"`
	GatewaySerials []string `json:"gatewaySerials"`
}

func adapterConfig(lease AdapterLease) (adapterSessionConfig, error) {
	var configured adapterSessionConfig
	if err := json.Unmarshal(lease.ConfigJSON, &configured); err != nil {
		return adapterSessionConfig{}, errors.New("DJI_ADAPTER_CONFIG_INVALID")
	}
	if len(configured.Topics) == 0 {
		return adapterSessionConfig{}, errors.New("DJI_ADAPTER_TOPICS_REQUIRED")
	}
	if len(configured.GatewaySerials) == 0 {
		return adapterSessionConfig{}, errors.New("DJI_ADAPTER_GATEWAYS_REQUIRED")
	}
	return configured, nil
}

func RouteContextFromLease(lease AdapterLease) (RouteContext, error) {
	configured, err := adapterConfig(lease)
	if err != nil {
		return RouteContext{}, err
	}
	allowed := make(map[string]bool, len(configured.GatewaySerials))
	for _, gatewaySN := range configured.GatewaySerials {
		gatewaySN = strings.TrimSpace(gatewaySN)
		if gatewaySN == "" {
			return RouteContext{}, errors.New("DJI_ADAPTER_GATEWAYS_REQUIRED")
		}
		allowed[gatewaySN] = true
	}
	return RouteContext{ProjectID: lease.ProjectID, AdapterID: lease.AdapterID, AllowedGatewaySNs: allowed}, nil
}

func BuildMQTTConfig(ctx context.Context, lease AdapterLease, resolver SecretResolver) (MQTTConfig, error) {
	if resolver == nil {
		return MQTTConfig{}, errors.New("DJI_SECRET_RESOLVER_REQUIRED")
	}
	configured, err := adapterConfig(lease)
	if err != nil {
		return MQTTConfig{}, err
	}
	if configured.ClientID == "" {
		configured.ClientID = fmt.Sprintf("aerosight-%d-%d", lease.ProjectID, lease.AdapterID)
	}
	credentials, err := resolver.ResolveMQTT(ctx, lease.SecretRef)
	if err != nil {
		return MQTTConfig{}, err
	}
	return MQTTConfig{
		BrokerURL: lease.BrokerURL, ClientID: configured.ClientID,
		Username: credentials.Username, Password: []byte(credentials.Password), Topics: configured.Topics,
	}, nil
}
