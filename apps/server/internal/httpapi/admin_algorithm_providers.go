package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const algorithmProviderPublicSQL = `select to_jsonb(p) from (select id::text,name,provider_type as "providerType",base_url as "baseUrl",auth_type as "authType",allowed_headers_json as "allowedHeaders",timeout_seconds as "timeoutSeconds",concurrency_limit as "concurrencyLimit",rate_limit_per_minute as "rateLimitPerMinute",status,health_json as health,updated_at as "updatedAt" from algorithm_providers where id=$1) p`

func (s *Server) algorithmProviderFailure(c *gin.Context, err error) {
	code, status := "ALGORITHM_PROVIDER_FAILED", 400
	switch err.Error() {
	case "ALGORITHM_PROVIDER_NOT_FOUND", "ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED", "ALGORITHM_PROVIDER_INPUT_INVALID", "ALGORITHM_PROVIDER_IN_USE", "OUTBOUND_URL_INVALID", "OUTBOUND_HTTPS_REQUIRED", "OUTBOUND_URL_CREDENTIALS_FORBIDDEN", "OUTBOUND_HOST_NOT_ALLOWED", "OUTBOUND_DNS_EMPTY", "OUTBOUND_DNS_FAILED", "OUTBOUND_ADDRESS_RESTRICTED":
		code = err.Error()
	default:
		if strings.HasPrefix(err.Error(), "ALGORITHM_ADAPTER_UNAVAILABLE:") {
			code = err.Error()
		}
	}
	s.failure(c, status, code)
}

func (s *Server) adminAlgorithmProviderRoutes() {
	g := s.router.Group("/api/admin/algorithm-providers", s.requireUser, s.requireAdmin, s.timeout)
	g.GET("", func(c *gin.Context) {
		rows, err := s.db.QueryContext(c.Request.Context(), `select to_jsonb(p) from (select id::text,name,provider_type as "providerType",base_url as "baseUrl",auth_type as "authType",allowed_headers_json as "allowedHeaders",timeout_seconds as "timeoutSeconds",concurrency_limit as "concurrencyLimit",rate_limit_per_minute as "rateLimitPerMinute",status,health_json as health,updated_at as "updatedAt" from algorithm_providers order by name,id) p`)
		if err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		defer rows.Close()
		result := []json.RawMessage{}
		for rows.Next() {
			var row json.RawMessage
			if err = rows.Scan(&row); err != nil {
				s.algorithmProviderFailure(c, err)
				return
			}
			result = append(result, row)
		}
		if err = rows.Err(); err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		c.JSON(200, result)
	})
	g.POST("", s.saveAdminAlgorithmProvider)
	g.PATCH("/:providerId", s.saveAdminAlgorithmProvider)
	g.POST("/:providerId/test", s.testAdminAlgorithmProvider)
	g.DELETE("/:providerId", s.deleteAdminAlgorithmProvider)
}

func adminAlgorithmProviderID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("providerId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
	}
	return id, nil
}

func (s *Server) saveAdminAlgorithmProvider(c *gin.Context) {
	creating := c.Request.Method == "POST"
	var id int64
	var err error
	if !creating {
		id, err = adminAlgorithmProviderID(c)
		if err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
	}
	var raw map[string]any
	if strictJSON(c, &raw) != nil {
		s.algorithmProviderFailure(c, errors.New("ALGORITHM_PROVIDER_INPUT_INVALID"))
		return
	}
	input, err := parseAlgorithmProvider(raw)
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	if creating && input.Params.AuthType != "none" && input.Credential == nil {
		s.algorithmProviderFailure(c, errors.New("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED"))
		return
	}
	ctx := c.Request.Context()
	if _, _, err = s.validateAlgorithmURL(ctx, input.Params.BaseUrl); err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	uid := currentUser(c).ID
	action := "algorithm_provider.create"
	if !creating {
		action = "algorithm_provider.update"
	}
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "algorithm_provider", Input: input.Audit}
	if !creating {
		audit.ResourceID = strconv.FormatInt(id, 10)
	}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		scope := 0
		hasCredential := false
		oldAuth := ""
		oldStatus := ""
		if !creating {
			var legacyScope sql.NullInt32
			e := w.Tx.QueryRowContext(ctx, `select project_id,auth_type,status,credential_envelope_json is not null from algorithm_providers where id=$1 for update`, id).Scan(&legacyScope, &oldAuth, &oldStatus, &hasCredential)
			if errors.Is(e, sql.ErrNoRows) {
				return nil, errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
			}
			if e != nil {
				return nil, e
			}
			if legacyScope.Valid {
				scope = int(legacyScope.Int32)
			}
			if input.Params.AuthType != "none" && input.Credential == nil && (!hasCredential || oldAuth != input.Params.AuthType) {
				return nil, errors.New("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED")
			}
		}
		headers := input.Params.AllowedHeadersJson
		if input.Status == "" {
			if creating {
				input.Status = "disabled"
			} else {
				input.Status = oldStatus
			}
		}
		if creating {
			e := w.Tx.QueryRowContext(ctx, `insert into algorithm_providers(name,provider_type,base_url,auth_type,allowed_headers_json,timeout_seconds,concurrency_limit,rate_limit_per_minute,status,created_by_user_id) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) returning id`, input.Params.Name, input.Params.ProviderType, input.Params.BaseUrl, input.Params.AuthType, headers, input.Params.TimeoutSeconds, input.Params.ConcurrencyLimit, input.Params.RateLimitPerMinute, input.Status, uid).Scan(&id)
			if e != nil {
				return nil, e
			}
		} else {
			_, e := w.Tx.ExecContext(ctx, `update algorithm_providers set name=$2,provider_type=$3,base_url=$4,auth_type=$5,allowed_headers_json=$6,timeout_seconds=$7,concurrency_limit=$8,rate_limit_per_minute=$9,status=$10,updated_at=now() where id=$1`, id, input.Params.Name, input.Params.ProviderType, input.Params.BaseUrl, input.Params.AuthType, headers, input.Params.TimeoutSeconds, input.Params.ConcurrencyLimit, input.Params.RateLimitPerMinute, input.Status)
			if e != nil {
				return nil, e
			}
		}
		if input.Credential != nil {
			envelope, e := credentials.EncryptJSON(input.Credential, s.credentialSecret, credentials.AAD("algorithm-provider", id, scope))
			if e != nil {
				return nil, e
			}
			encoded, e := json.Marshal(envelope)
			if e != nil {
				return nil, e
			}
			if _, e = w.Tx.ExecContext(ctx, `update algorithm_providers set credential_envelope_json=$2 where id=$1`, id, encoded); e != nil {
				return nil, e
			}
		}
		var public json.RawMessage
		if e := w.Tx.QueryRowContext(ctx, algorithmProviderPublicSQL, id).Scan(&public); e != nil {
			return nil, e
		}
		var value gin.H
		if e := json.Unmarshal(public, &value); e != nil {
			return nil, e
		}
		return value, nil
	})
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	status := 200
	if creating {
		status = 201
	}
	c.JSON(status, result)
}

func (s *Server) testAdminAlgorithmProvider(c *gin.Context) {
	id, err := adminAlgorithmProviderID(c)
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	var endpoint, kind string
	err = s.db.QueryRowContext(c.Request.Context(), `select base_url,provider_type from algorithm_providers where id=$1`, id).Scan(&endpoint, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
	}
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	capability, err := algorithm.RequireEnabled(kind)
	if err != nil {
		s.algorithmProviderFailure(c, errors.New("ALGORITHM_ADAPTER_UNAVAILABLE:"+kind))
		return
	}
	target, count, err := s.validateAlgorithmURL(c.Request.Context(), endpoint)
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"safe": true, "providerType": kind, "capability": capability, "protocol": target.Scheme + ":", "host": target.Hostname(), "resolvedAddressCount": count})
}

func (s *Server) deleteAdminAlgorithmProvider(c *gin.Context) {
	id, err := adminAlgorithmProviderID(c)
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	uid := currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "algorithm_provider.delete", ResourceType: "algorithm_provider", ResourceID: strconv.FormatInt(id, 10)}
	_, err = database.AuditedPlatformWrite(c.Request.Context(), s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		var used bool
		if e := w.Tx.QueryRowContext(c.Request.Context(), `select exists(select 1 from algorithm_definitions where provider_id=$1)`, id).Scan(&used); e != nil {
			return nil, e
		}
		if used {
			return nil, errors.New("ALGORITHM_PROVIDER_IN_USE")
		}
		result, e := w.Tx.ExecContext(c.Request.Context(), `delete from algorithm_providers where id=$1`, id)
		if e != nil {
			return nil, e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return nil, e
		}
		if count == 0 {
			return nil, errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
		}
		return gin.H{"deleted": true}, nil
	})
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"deleted": true})
}
