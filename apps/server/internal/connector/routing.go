package connector

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

var (
	ErrRouteUnavailable = errors.New("connector route unavailable")
	ErrRouteConflict    = errors.New("connector route has multiple primary bindings")
)

type BindingRoute struct {
	ConnectorInstanceID int64
	ExternalIdentityID  int64
	Role                string
	Priority            int
	Status              string
}

// SelectPrimaryRoute fails closed when the highest-priority active route is not unique.
// Standby, disabled, and conflicted bindings are never eligible for command dispatch.
func SelectPrimaryRoute(routes []BindingRoute) (BindingRoute, error) {
	eligible := make([]BindingRoute, 0, len(routes))
	for _, route := range routes {
		if route.Status != "active" || route.ConnectorInstanceID <= 0 || route.ExternalIdentityID <= 0 {
			continue
		}
		switch route.Role {
		case "direct", "gateway", "inherited":
			eligible = append(eligible, route)
		}
	}
	if len(eligible) == 0 {
		return BindingRoute{}, ErrRouteUnavailable
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Priority == eligible[j].Priority {
			return eligible[i].ConnectorInstanceID < eligible[j].ConnectorInstanceID
		}
		return eligible[i].Priority > eligible[j].Priority
	})
	if len(eligible) > 1 && eligible[0].Priority == eligible[1].Priority {
		return BindingRoute{}, ErrRouteConflict
	}
	return eligible[0], nil
}

func ResolvePrimaryBinding(ctx context.Context, tx *sql.Tx, projectID, deviceID int) (BindingRoute, error) {
	rows, err := tx.QueryContext(ctx, `
		select binding.connector_instance_id,binding.external_identity_id,
		       binding.route_role,binding.priority,binding.status
		  from device_connector_bindings binding
		  join device_adapters adapter
		    on adapter.id=binding.connector_instance_id and adapter.project_id=binding.project_id
		 where binding.project_id=$1 and binding.device_id=$2
		   and binding.status='active' and adapter.status='connected'
		 order by binding.priority desc,binding.connector_instance_id`, projectID, deviceID)
	if err != nil {
		return BindingRoute{}, err
	}
	defer rows.Close()
	routes := []BindingRoute{}
	for rows.Next() {
		var route BindingRoute
		if err := rows.Scan(&route.ConnectorInstanceID, &route.ExternalIdentityID, &route.Role, &route.Priority, &route.Status); err != nil {
			return BindingRoute{}, err
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return BindingRoute{}, err
	}
	return SelectPrimaryRoute(routes)
}
