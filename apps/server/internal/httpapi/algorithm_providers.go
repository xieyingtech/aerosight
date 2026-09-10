package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sqlc-dev/pqtype"
)

func (s *Server) algorithmProviderFailure(c *gin.Context, err error) {
	code, status := "ALGORITHM_PROVIDER_FAILED", 400
	switch err.Error() {
	case "PROJECT_ACCESS_DENIED":
		code, status = err.Error(), 403
	case "ALGORITHM_PROVIDER_NOT_FOUND", "ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED", "ALGORITHM_PROVIDER_INPUT_INVALID", "OUTBOUND_URL_INVALID", "OUTBOUND_HTTPS_REQUIRED", "OUTBOUND_URL_CREDENTIALS_FORBIDDEN", "OUTBOUND_HOST_NOT_ALLOWED", "OUTBOUND_DNS_EMPTY", "OUTBOUND_DNS_FAILED", "OUTBOUND_ADDRESS_RESTRICTED":
		code = err.Error()
	default:
		if strings.HasPrefix(err.Error(), "ALGORITHM_ADAPTER_UNAVAILABLE:") {
			code = err.Error()
		}
	}
	s.failure(c, status, code)
}

func (s *Server) algorithmProviderRoutes() {
	group := s.router.Group("/api/projects/:id/algorithm-providers", s.requireUser, s.timeout)
	group.POST("", s.saveAlgorithmProvider)
	group.PATCH("/:providerId", s.saveAlgorithmProvider)
	group.POST("/:providerId/test", s.testAlgorithmProvider)
	group.GET("", func(c *gin.Context) {
		pid, err := projectID(c)
		if err != nil {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			return
		}
		tx, err := s.db.BeginTx(c.Request.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		defer tx.Rollback()
		q := s.queries.WithTx(tx)
		if _, err = s.projectAccess(c.Request.Context(), q, currentUser(c).ID, pid, "algorithm:manage"); err != nil {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			return
		}
		raw, err := q.ListAlgorithmProviders(c.Request.Context(), pid)
		if err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		rows, err := decodeSnapshotRows(raw)
		if err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		if err = tx.Commit(); err != nil {
			s.algorithmProviderFailure(c, err)
			return
		}
		c.JSON(200, rows)
	})
}

func (s *Server) saveAlgorithmProvider(c *gin.Context) {
	bad := func() { s.failure(c, 400, "ALGORITHM_PROVIDER_INPUT_INVALID") }
	pid, err := projectID(c)
	if err != nil {
		bad()
		return
	}
	creating := c.Request.Method == "POST"
	var id int64
	if !creating {
		id, err = strconv.ParseInt(c.Param("providerId"), 10, 64)
		if err != nil || id <= 0 {
			bad()
			return
		}
	}
	var raw map[string]any
	if strictJSON(c, &raw) != nil {
		bad()
		return
	}
	input, err := parseAlgorithmProvider(raw)
	if err != nil {
		bad()
		return
	}
	if creating && input.Params.AuthType != "none" && input.Credential == nil {
		s.algorithmProviderFailure(c, errors.New("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED"))
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "algorithm:manage")
	if err != nil {
		s.algorithmProviderFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	if _, _, err = s.validateAlgorithmURL(ctx, input.Params.BaseUrl); err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	action, resource, flag := "algorithm_provider.create", "", "encryptedCredential"
	if !creating {
		action, resource, flag = "algorithm_provider.update", strconv.FormatInt(id, 10), "credentialUpdated"
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "algorithm_provider", ResourceID: resource, Input: input.Audit, PolicyResult: map[string]any{"permission": "algorithm:manage", flag: input.Credential != nil}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "algorithm:manage", false), func(w *database.WriteTx) (gin.H, error) {
		p := input.Params
		p.ProjectID = pid
		p.TeamID = a.TeamID
		p.CreatedByUserID = sql.NullInt32{Int32: uid, Valid: true}
		if creating {
			id, err = w.Queries.CreateAlgorithmProvider(ctx, p)
			if err != nil {
				return nil, err
			}
		} else {
			current, e := w.Queries.LockAlgorithmProviderCredential(ctx, sqlcgen.LockAlgorithmProviderCredentialParams{ProjectID: pid, ID: id})
			if errors.Is(e, sql.ErrNoRows) {
				return nil, errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
			}
			if e != nil {
				return nil, e
			}
			if p.AuthType != "none" && input.Credential == nil && (!current.HasCredential || current.AuthType != p.AuthType) {
				return nil, errors.New("ALGORITHM_PROVIDER_CREDENTIAL_REQUIRED")
			}
			e = w.Queries.UpdateAlgorithmProvider(ctx, sqlcgen.UpdateAlgorithmProviderParams{ProjectID: pid, ID: id, Name: p.Name, ProviderType: p.ProviderType, BaseUrl: p.BaseUrl, AuthType: p.AuthType, AllowedHeadersJson: p.AllowedHeadersJson, TimeoutSeconds: p.TimeoutSeconds, ConcurrencyLimit: p.ConcurrencyLimit, RateLimitPerMinute: p.RateLimitPerMinute})
			if e != nil {
				return nil, e
			}
		}
		if input.Credential != nil {
			envelope, e := credentials.EncryptJSON(input.Credential, s.credentialSecret, credentials.AAD("algorithm-provider", id, int(pid)))
			if e != nil {
				return nil, e
			}
			encoded, e := json.Marshal(envelope)
			if e != nil {
				return nil, e
			}
			e = w.Queries.SetAlgorithmProviderCredential(ctx, sqlcgen.SetAlgorithmProviderCredentialParams{ProjectID: pid, ID: id, CredentialEnvelopeJson: pqtype.NullRawMessage{RawMessage: encoded, Valid: true}})
			if e != nil {
				return nil, e
			}
		}
		raw, e := w.Queries.ReadAlgorithmProviderPublic(ctx, sqlcgen.ReadAlgorithmProviderPublicParams{ProjectID: pid, ID: id})
		if e != nil {
			return nil, e
		}
		rows, e := decodeSnapshotRows([]json.RawMessage{raw})
		if e != nil {
			return nil, e
		}
		return rows[0], nil
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

func (s *Server) testAlgorithmProvider(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.failure(c, 400, "ALGORITHM_PROVIDER_NOT_FOUND")
		return
	}
	id, err := strconv.ParseInt(c.Param("providerId"), 10, 64)
	if err != nil || id <= 0 {
		s.failure(c, 400, "ALGORITHM_PROVIDER_NOT_FOUND")
		return
	}
	ctx := c.Request.Context()
	if _, err = s.projectAccess(ctx, s.queries, currentUser(c).ID, pid, "algorithm:manage"); err != nil {
		s.algorithmProviderFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	row, err := s.queries.ReadAlgorithmProviderEndpoint(ctx, sqlcgen.ReadAlgorithmProviderEndpointParams{ProjectID: pid, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
	}
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	capability, err := algorithm.RequireEnabled(row.ProviderType)
	if err != nil {
		s.algorithmProviderFailure(c, errors.New("ALGORITHM_ADAPTER_UNAVAILABLE:"+row.ProviderType))
		return
	}
	target, count, err := s.validateAlgorithmURL(ctx, row.BaseUrl)
	if err != nil {
		s.algorithmProviderFailure(c, err)
		return
	}
	if _, err = s.projectAccess(ctx, s.queries, currentUser(c).ID, pid, "algorithm:manage"); err != nil {
		s.algorithmProviderFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	c.JSON(200, gin.H{"safe": true, "providerType": row.ProviderType, "capability": gin.H{"providerType": capability.ProviderType, "implementationStatus": capability.ImplementationStatus, "executionModes": capability.ExecutionModes, "supportsPolling": capability.SupportsPolling, "supportsSignedCallbacks": capability.SupportsSignedCallbacks, "contractVersion": capability.ContractVersion, "unavailableReason": nil}, "protocol": target.Scheme + ":", "host": target.Hostname(), "resolvedAddressCount": count})
}
