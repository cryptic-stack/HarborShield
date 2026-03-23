package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"harborshield/backend/internal/adminapi"
	"harborshield/backend/internal/audit"
	"harborshield/backend/internal/auth"
	"harborshield/backend/internal/authz"
	"harborshield/backend/internal/buckets"
	"harborshield/backend/internal/config"
	"harborshield/backend/internal/credentials"
	"harborshield/backend/internal/dashboard"
	"harborshield/backend/internal/events"
	"harborshield/backend/internal/malware"
	"harborshield/backend/internal/middleware"
	"harborshield/backend/internal/objects"
	"harborshield/backend/internal/oidc"
	"harborshield/backend/internal/policies"
	"harborshield/backend/internal/quotas"
	"harborshield/backend/internal/settings"
	"harborshield/backend/internal/storage"
	"harborshield/backend/internal/users"
)

type RouterDeps struct {
	Auth             *auth.Service
	Authorizer       *authz.Authorizer
	AdminTokens      *adminapi.Service
	Buckets          *buckets.Service
	Objects          *objects.Service
	OIDC             *oidc.Service
	Credentials      *credentials.Service
	Dashboard        *dashboard.Service
	Users            *users.Service
	Audit            *audit.Service
	Events           *events.Service
	Malware          *malware.Service
	Policies         *policies.Service
	Quotas           *quotas.Service
	Settings         *settings.Service
	StorageTopology  *storage.TopologyService
	Tokens           auth.TokenManager
	AdminIPAllowlist []string
}

func Mount(r chi.Router, deps RouterDeps) {
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.AdminIPAllowlist(deps.AdminIPAllowlist))
		r.Post("/auth/login", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			response, err := deps.Auth.Login(req.Context(), body.Email, body.Password)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidCredentials) {
					middleware.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
					return
				}
				middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "login failed"})
				return
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     body.Email,
				Action:    "auth.login",
				Resource:  "session",
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
				Detail:    map[string]any{"email": body.Email},
			})
			middleware.WriteJSON(w, http.StatusOK, response)
		})

		r.Post("/auth/refresh", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				RefreshToken string `json:"refreshToken"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			response, err := deps.Auth.Refresh(req.Context(), body.RefreshToken)
			if err != nil {
				middleware.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     "refresh-token",
				Action:    "auth.refresh",
				Resource:  "session",
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
			})
			middleware.WriteJSON(w, http.StatusOK, response)
		})

		r.Post("/auth/logout", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				RefreshToken string `json:"refreshToken"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			if body.RefreshToken == "" {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "refreshToken is required"})
				return
			}
			if err := deps.Auth.Logout(req.Context(), body.RefreshToken); err != nil {
				middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "logout failed"})
				return
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     "refresh-token",
				Action:    "auth.logout",
				Resource:  "session",
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
			})
			middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
		})

		r.Get("/auth/oidc", func(w http.ResponseWriter, req *http.Request) {
			middleware.WriteJSON(w, http.StatusOK, deps.OIDC.Status(req.Context()))
		})

		r.Post("/internal/storage/join", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Token    string `json:"token"`
				Name     string `json:"name"`
				Endpoint string `json:"endpoint"`
				Zone     string `json:"zone"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			item, err := deps.StorageTopology.JoinNode(req.Context(), body.Token, body.Name, body.Endpoint, body.Zone)
			if err != nil {
				status := http.StatusInternalServerError
				switch {
				case errors.Is(err, storage.ErrJoinTokenInvalid), errors.Is(err, storage.ErrJoinTokenUsed):
					status = http.StatusUnauthorized
				default:
					status = http.StatusBadRequest
				}
				middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			middleware.WriteJSON(w, http.StatusCreated, item)
		})

		r.Post("/auth/oidc/start", func(w http.ResponseWriter, req *http.Request) {
			startURL, err := deps.OIDC.Start(req.Context())
			if err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			middleware.WriteJSON(w, http.StatusOK, map[string]string{"url": startURL})
		})

		r.Get("/auth/oidc/start", func(w http.ResponseWriter, req *http.Request) {
			startURL, err := deps.OIDC.Start(req.Context())
			if err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			http.Redirect(w, req, startURL, http.StatusFound)
		})

		r.Get("/auth/oidc/callback", func(w http.ResponseWriter, req *http.Request) {
			page, err := deps.OIDC.Callback(req.Context(), req.URL.Query().Get("code"), req.URL.Query().Get("state"))
			if err != nil {
				middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, page)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminAuth(deps.Tokens, deps.AdminTokens))

			r.Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
				middleware.WriteJSON(w, http.StatusOK, middleware.ClaimsFromContext(req.Context()))
			})

			r.Get("/setup/status", func(w http.ResponseWriter, req *http.Request) {
				status, err := deps.Settings.DeploymentSetupStatus(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, status)
			})

			r.Patch("/settings/oidc", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				before, err := deps.Settings.ResolveOIDCSettings(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				var body settings.OIDCSettingsInput
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.Settings.UpdateOIDCSettings(req.Context(), body)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "settings.oidc.update",
					Resource:  "oidc",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"before": map[string]any{
							"enabled":                before.Enabled,
							"issuerUrl":              before.IssuerURL,
							"clientId":               before.ClientID,
							"redirectUrl":            before.RedirectURL,
							"clientSecretConfigured": before.ClientSecretConfigured,
							"scopes":                 before.Scopes,
							"roleClaim":              before.RoleClaim,
							"defaultRole":            before.DefaultRole,
							"roleMap":                before.RoleMap,
						},
						"after": map[string]any{
							"enabled":                item.Enabled,
							"issuerUrl":              item.IssuerURL,
							"clientId":               item.ClientID,
							"redirectUrl":            item.RedirectURL,
							"clientSecretConfigured": item.ClientSecretConfigured,
							"scopes":                 item.Scopes,
							"roleClaim":              item.RoleClaim,
							"defaultRole":            item.DefaultRole,
							"roleMap":                item.RoleMap,
						},
						"changedFields": diffSettingsFields(
							map[string]any{
								"enabled":                before.Enabled,
								"issuerUrl":              before.IssuerURL,
								"clientId":               before.ClientID,
								"redirectUrl":            before.RedirectURL,
								"clientSecretConfigured": before.ClientSecretConfigured,
								"scopes":                 before.Scopes,
								"roleClaim":              before.RoleClaim,
								"defaultRole":            before.DefaultRole,
								"roleMap":                before.RoleMap,
							},
							map[string]any{
								"enabled":                item.Enabled,
								"issuerUrl":              item.IssuerURL,
								"clientId":               item.ClientID,
								"redirectUrl":            item.RedirectURL,
								"clientSecretConfigured": item.ClientSecretConfigured,
								"scopes":                 item.Scopes,
								"roleClaim":              item.RoleClaim,
								"defaultRole":            item.DefaultRole,
								"roleMap":                item.RoleMap,
							},
						),
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Patch("/settings/malware", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				before, err := deps.Settings.ResolveMalwareSettings(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				var body settings.MalwareSettings
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.Settings.UpdateMalwareSettings(req.Context(), body)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "settings.malware.update",
					Resource:  "malware",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"before": map[string]any{
							"scanMode": before.ScanMode,
						},
						"after": map[string]any{
							"scanMode": item.ScanMode,
						},
						"changedFields": diffSettingsFields(
							map[string]any{"scanMode": before.ScanMode},
							map[string]any{"scanMode": item.ScanMode},
						),
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/settings/oidc/clear-secret", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				before, err := deps.Settings.ResolveOIDCSettings(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				item, err := deps.Settings.ClearOIDCClientSecret(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "settings.oidc.clear-secret",
					Resource:  "oidc",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"before": map[string]any{
							"clientSecretConfigured": before.ClientSecretConfigured,
						},
						"after": map[string]any{
							"clientSecretConfigured": item.ClientSecretConfigured,
						},
						"changedFields": []string{"clientSecretConfigured"},
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/settings/oidc/test", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				item, err := deps.OIDC.TestConnection(req.Context())
				if err != nil {
					_ = deps.Audit.Record(req.Context(), audit.Entry{
						Actor:     claims.Email,
						Action:    "settings.oidc.test",
						Resource:  "oidc",
						Outcome:   "failure",
						RequestID: req.Header.Get("X-Request-Id"),
						Detail:    map[string]any{"error": err.Error()},
					})
					status := http.StatusBadRequest
					if strings.HasPrefix(strings.ToLower(err.Error()), "get ") || strings.Contains(strings.ToLower(err.Error()), "oidc:") {
						status = http.StatusBadGateway
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "settings.oidc.test",
					Resource:  "oidc",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"issuerUrl":             item.IssuerURL,
						"authorizationEndpoint": item.AuthorizationEndpoint,
						"tokenEndpoint":         item.TokenEndpoint,
						"jwksUrl":               item.JWKSURL,
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/setup/complete", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				var body settings.DeploymentSetupInput
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				status, err := deps.Settings.CompleteDeploymentSetup(req.Context(), body)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "setup.complete",
					Resource:  "deployment",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"desiredStorageBackend": status.DesiredStorageBackend,
						"distributedScope":      status.DistributedScope,
						"remoteEndpoints":       status.RemoteEndpoints,
						"applyRequired":         status.ApplyRequired,
					},
				})
				middleware.WriteJSON(w, http.StatusOK, status)
			})

			r.Post("/auth/change-password", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				var body struct {
					CurrentPassword string `json:"currentPassword"`
					NewPassword     string `json:"newPassword"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.CurrentPassword == "" || body.NewPassword == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "currentPassword and newPassword are required"})
					return
				}
				if err := deps.Auth.ChangePassword(req.Context(), claims.UserID, body.CurrentPassword, body.NewPassword); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "auth.change-password",
					Resource:  claims.Email,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
				})
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			})

			r.Post("/auth/logout-all", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if err := deps.Auth.LogoutAll(req.Context(), claims.UserID); err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "logout failed"})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "auth.logout-all",
					Resource:  claims.Email,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
				})
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
			})

			r.Get("/buckets", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "bucket.list", "*") {
					return
				}
				list, err := deps.Buckets.List(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": list})
			})

			r.Post("/buckets", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "bucket.create", "*") {
					return
				}
				var body struct {
					Name          string `json:"name"`
					StorageClass  string `json:"storageClass"`
					ReplicaTarget int    `json:"replicaTarget"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.Buckets.Create(req.Context(), body.Name, "default", claims.UserID, body.StorageClass, body.ReplicaTarget)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "bucket.create",
					Resource:  body.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": item.ID, "storageClass": item.StorageClass, "replicaTarget": item.ReplicaTarget, "effectiveStorageClass": item.EffectiveStorageClass, "effectiveReplicaTarget": item.EffectiveReplicaTarget},
				})
				_ = deps.Events.Emit(req.Context(), "bucket.created", map[string]any{"bucketId": item.ID, "bucketName": item.Name})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Get("/buckets/{bucketID}/objects", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "object.list", "bucket:"+chi.URLParam(req, "bucketID")) {
					return
				}
				prefix := req.URL.Query().Get("prefix")
				items, err := deps.Objects.List(req.Context(), chi.URLParam(req, "bucketID"), prefix)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/buckets/{bucketID}", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "bucket.list", "*") {
					return
				}
				item, err := deps.Buckets.GetByID(req.Context(), chi.URLParam(req, "bucketID"))
				if err != nil {
					middleware.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Patch("/buckets/{bucketID}/durability", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "bucket.create", "*") {
					return
				}
				var body struct {
					StorageClass  string `json:"storageClass"`
					ReplicaTarget int    `json:"replicaTarget"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.Buckets.UpdateDurability(req.Context(), chi.URLParam(req, "bucketID"), body.StorageClass, body.ReplicaTarget)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "bucket.durability.update",
					Resource:  item.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": item.ID, "storageClass": item.StorageClass, "replicaTarget": item.ReplicaTarget},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Get("/buckets/{bucketID}/objects/versions", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "object.list", "bucket:"+chi.URLParam(req, "bucketID")) {
					return
				}
				key := req.URL.Query().Get("key")
				if key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				items, err := deps.Objects.ListVersions(req.Context(), chi.URLParam(req, "bucketID"), key)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/buckets/{bucketID}/objects/restore", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.restore", "bucket:"+bucketID) {
					return
				}
				var body struct {
					Key string `json:"key"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				item, err := deps.Objects.RestoreLatest(req.Context(), bucketID, body.Key)
				if err != nil {
					status := http.StatusInternalServerError
					if errors.Is(err, objects.ErrNoRestorableVersion) {
						status = http.StatusConflict
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.restore",
					Resource:  body.Key,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID, "versionId": item.VersionID},
				})
				_ = deps.Events.Emit(req.Context(), "object.restored", map[string]any{"bucketId": bucketID, "objectId": item.ID, "key": body.Key, "versionId": item.VersionID})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Get("/buckets/{bucketID}/objects/tags", func(w http.ResponseWriter, req *http.Request) {
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.get", "bucket:"+bucketID) {
					return
				}
				key := req.URL.Query().Get("key")
				if key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				tags, err := deps.Objects.GetTags(req.Context(), bucketID, key, req.URL.Query().Get("versionId"))
				if err != nil {
					middleware.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
			})

			r.Put("/buckets/{bucketID}/objects/tags", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.put", "bucket:"+bucketID) {
					return
				}
				var body struct {
					Key       string            `json:"key"`
					VersionID string            `json:"versionId"`
					Tags      map[string]string `json:"tags"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				item, err := deps.Objects.PutTags(req.Context(), bucketID, body.Key, body.VersionID, body.Tags)
				if err != nil {
					middleware.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.tagging.put",
					Resource:  body.Key,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID, "versionId": item.VersionID, "tagCount": len(item.Tags)},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Put("/buckets/{bucketID}/objects/legal-hold", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.delete", "bucket:"+bucketID) {
					return
				}
				var body struct {
					Key       string `json:"key"`
					VersionID string `json:"versionId"`
					LegalHold bool   `json:"legalHold"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				item, err := deps.Objects.SetLegalHold(req.Context(), bucketID, body.Key, body.VersionID, body.LegalHold)
				if err != nil {
					middleware.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.legal-hold.put",
					Resource:  body.Key,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID, "versionId": item.VersionID, "legalHold": item.LegalHold},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/buckets/{bucketID}/objects/copy", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.put", "bucket:"+bucketID) {
					return
				}
				var body struct {
					SourceBucketID     string            `json:"sourceBucketId"`
					SourceKey          string            `json:"sourceKey"`
					SourceVersionID    string            `json:"sourceVersionId"`
					DestinationKey     string            `json:"destinationKey"`
					ReplaceMetadata    bool              `json:"replaceMetadata"`
					ReplaceTags        bool              `json:"replaceTags"`
					ContentType        string            `json:"contentType"`
					CacheControl       string            `json:"cacheControl"`
					ContentDisposition string            `json:"contentDisposition"`
					ContentEncoding    string            `json:"contentEncoding"`
					UserMetadata       map[string]string `json:"userMetadata"`
					Tags               map[string]string `json:"tags"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.SourceBucketID == "" || body.SourceKey == "" || body.DestinationKey == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "sourceBucketId, sourceKey, and destinationKey are required"})
					return
				}
				if !authorize(w, req, deps.Authorizer, "object.get", "bucket:"+body.SourceBucketID) {
					return
				}
				input := objects.PutInput{
					Key:       body.DestinationKey,
					CreatedBy: claims.UserID,
				}
				if body.ReplaceMetadata {
					input.ContentType = body.ContentType
					input.CacheControl = body.CacheControl
					input.ContentDisposition = body.ContentDisposition
					input.ContentEncoding = body.ContentEncoding
					input.UserMetadata = body.UserMetadata
				}
				if body.ReplaceTags {
					input.Tags = body.Tags
				}
				item, err := deps.Objects.Copy(req.Context(), body.SourceBucketID, body.SourceKey, body.SourceVersionID, bucketID, input)
				if err != nil {
					status := http.StatusBadRequest
					if quotas.IsQuotaExceeded(err) {
						status = http.StatusConflict
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				applyQuotaWarningHeaders(w, req.Context(), deps.Quotas, bucketID, claims.UserID)
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.copy",
					Resource:  body.DestinationKey,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID, "versionId": item.VersionID, "sourceBucketId": body.SourceBucketID, "sourceKey": body.SourceKey},
				})
				_ = deps.Events.Emit(req.Context(), "object.copied", map[string]any{"bucketId": bucketID, "objectId": item.ID, "key": body.DestinationKey, "sourceBucketId": body.SourceBucketID, "sourceKey": body.SourceKey})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Post("/buckets/{bucketID}/objects/upload", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.put", "bucket:"+bucketID) {
					return
				}
				if err := req.ParseMultipartForm(32 << 20); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart payload"})
					return
				}
				file, header, err := req.FormFile("file")
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
					return
				}
				defer file.Close()

				key := req.FormValue("key")
				if key == "" {
					key = header.Filename
				}
				contentType := header.Header.Get("Content-Type")
				if contentType == "" {
					contentType = req.FormValue("contentType")
				}
				input, err := objectPutInput(file, header, key, contentType, claims.UserID)
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				item, err := deps.Objects.Put(req.Context(), bucketID, input)
				if err != nil {
					status := http.StatusBadRequest
					if quotas.IsQuotaExceeded(err) {
						status = http.StatusConflict
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				applyQuotaWarningHeaders(w, req.Context(), deps.Quotas, bucketID, claims.UserID)
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.put",
					Resource:  key,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID, "objectId": item.ID},
				})
				_ = deps.Events.Emit(req.Context(), "object.created", map[string]any{"bucketId": bucketID, "objectId": item.ID, "key": key})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Get("/buckets/{bucketID}/objects/download", func(w http.ResponseWriter, req *http.Request) {
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.get", "bucket:"+bucketID) {
					return
				}
				key := req.URL.Query().Get("key")
				if key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				versionID := req.URL.Query().Get("versionId")
				var item objects.Metadata
				var reader io.ReadCloser
				var err error
				if versionID == "" {
					item, reader, err = deps.Objects.GetByKey(req.Context(), bucketID, key)
				} else {
					item, reader, err = deps.Objects.GetByVersion(req.Context(), bucketID, key, versionID)
				}
				if err != nil {
					middleware.WriteJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
					return
				}
				defer reader.Close()
				w.Header().Set("Content-Type", item.ContentType)
				w.Header().Set("Content-Length", strconv.FormatInt(item.SizeBytes, 10))
				w.Header().Set("ETag", `"`+item.ETag+`"`)
				if item.ContentDisposition != "" {
					w.Header().Set("Content-Disposition", item.ContentDisposition)
				} else {
					w.Header().Set("Content-Disposition", `attachment; filename="`+item.Key+`"`)
				}
				if item.CacheControl != "" {
					w.Header().Set("Cache-Control", item.CacheControl)
				}
				if item.ContentEncoding != "" {
					w.Header().Set("Content-Encoding", item.ContentEncoding)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = io.Copy(w, reader)
			})

			r.Delete("/buckets/{bucketID}/objects", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				bucketID := chi.URLParam(req, "bucketID")
				if !authorize(w, req, deps.Authorizer, "object.delete", "bucket:"+bucketID) {
					return
				}
				key := req.URL.Query().Get("key")
				if key == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
					return
				}
				if err := deps.Objects.Delete(req.Context(), bucketID, key); err != nil {
					status := http.StatusNotFound
					if errors.Is(err, objects.ErrRetentionActive) || errors.Is(err, objects.ErrLegalHoldActive) {
						status = http.StatusConflict
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "object.delete",
					Resource:  key,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucketID},
				})
				_ = deps.Events.Emit(req.Context(), "object.deleted", map[string]any{"bucketId": bucketID, "key": key})
				w.WriteHeader(http.StatusNoContent)
			})

			r.Get("/users", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "user.manage", "*") {
					return
				}
				items, err := deps.Users.List(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/credentials", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "credential.create", "*") {
					return
				}
				var body struct {
					UserID      string `json:"userId"`
					Role        string `json:"role"`
					Description string `json:"description"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.Credentials.Create(req.Context(), body.UserID, body.Role, body.Description)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "credential.create",
					Resource:  body.Role,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"accessKey":   item["accessKey"],
						"userId":      body.UserID,
						"role":        body.Role,
						"description": body.Description,
					},
				})
				_ = deps.Events.Emit(req.Context(), "credential.created", map[string]any{"userId": body.UserID, "role": body.Role, "description": body.Description})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Get("/audit", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "audit.read", "*") {
					return
				}
				limit := 100
				if raw := req.URL.Query().Get("limit"); raw != "" {
					parsed, err := strconv.Atoi(raw)
					if err != nil {
						middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
						return
					}
					limit = parsed
				}
				items, err := deps.Audit.List(req.Context(), audit.ListFilter{
					Actor:    req.URL.Query().Get("actor"),
					Action:   req.URL.Query().Get("action"),
					Outcome:  req.URL.Query().Get("outcome"),
					Category: req.URL.Query().Get("category"),
					Severity: req.URL.Query().Get("severity"),
					Query:    req.URL.Query().Get("query"),
					Limit:    limit,
				})
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "health.read", "*") {
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			})

			r.Get("/dashboard", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "health.read", "*") {
					return
				}
				summary, err := deps.Dashboard.Summary(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, summary)
			})

			r.Get("/event-targets", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "event.read", "*") {
					return
				}
				items, err := deps.Events.ListTargets(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/event-targets", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "event.manage", "*") {
					return
				}
				var body struct {
					Name          string   `json:"name"`
					EndpointURL   string   `json:"endpointUrl"`
					SigningSecret string   `json:"signingSecret"`
					EventTypes    []string `json:"eventTypes"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Name == "" || body.EndpointURL == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "name and endpointUrl are required"})
					return
				}
				item, err := deps.Events.CreateTarget(req.Context(), body.Name, body.EndpointURL, body.SigningSecret, body.EventTypes)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "event-target.create",
					Resource:  item.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"targetId":     item.ID,
						"targetType":   item.TargetType,
						"endpointUrl":  item.EndpointURL,
						"eventTypes":   item.EventTypes,
						"secretStored": body.SigningSecret != "",
					},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Get("/event-deliveries", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "event.read", "*") {
					return
				}
				limit := 100
				if raw := req.URL.Query().Get("limit"); raw != "" {
					parsed, err := strconv.Atoi(raw)
					if err != nil {
						middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
						return
					}
					limit = parsed
				}
				items, err := deps.Events.ListDeliveries(req.Context(), events.DeliveryFilters{
					Status:    req.URL.Query().Get("status"),
					TargetID:  req.URL.Query().Get("targetId"),
					EventType: req.URL.Query().Get("eventType"),
					Limit:     limit,
				})
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/malware-status", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "malware.read", "*") {
					return
				}
				limit := 100
				if raw := req.URL.Query().Get("limit"); raw != "" {
					parsed, err := strconv.Atoi(raw)
					if err != nil {
						middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
						return
					}
					limit = parsed
				}
				items, err := deps.Malware.ListRecent(req.Context(), limit)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/settings", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "settings.read", "*") {
					return
				}
				snapshot, err := deps.Settings.Snapshot(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, snapshot)
			})

			r.Patch("/settings/storage-policy", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "settings.manage", "*") {
					return
				}
				before, err := deps.Settings.ResolveStoragePolicy(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				var body struct {
					DefaultStorageClass       string `json:"defaultStorageClass"`
					StandardReplicas          int    `json:"standardReplicas"`
					ReducedRedundancyReplicas int    `json:"reducedRedundancyReplicas"`
					ArchiveReadyReplicas      int    `json:"archiveReadyReplicas"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				policy, err := deps.Settings.UpdateStoragePolicy(req.Context(), body.DefaultStorageClass, map[string]int{
					"standard":           body.StandardReplicas,
					"reduced-redundancy": body.ReducedRedundancyReplicas,
					"archive-ready":      body.ArchiveReadyReplicas,
				})
				if err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "settings.storage-policy.update",
					Resource:  "cluster-storage-policy",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"before": map[string]any{
							"defaultStorageClass":       before.DefaultClass,
							"standardReplicas":          replicaForPolicy(before.Policies, "standard"),
							"reducedRedundancyReplicas": replicaForPolicy(before.Policies, "reduced-redundancy"),
							"archiveReadyReplicas":      replicaForPolicy(before.Policies, "archive-ready"),
						},
						"after": map[string]any{
							"defaultStorageClass":       policy.DefaultClass,
							"standardReplicas":          replicaForPolicy(policy.Policies, "standard"),
							"reducedRedundancyReplicas": replicaForPolicy(policy.Policies, "reduced-redundancy"),
							"archiveReadyReplicas":      replicaForPolicy(policy.Policies, "archive-ready"),
						},
						"changedFields": diffSettingsFields(
							map[string]any{
								"defaultStorageClass":       before.DefaultClass,
								"standardReplicas":          replicaForPolicy(before.Policies, "standard"),
								"reducedRedundancyReplicas": replicaForPolicy(before.Policies, "reduced-redundancy"),
								"archiveReadyReplicas":      replicaForPolicy(before.Policies, "archive-ready"),
							},
							map[string]any{
								"defaultStorageClass":       policy.DefaultClass,
								"standardReplicas":          replicaForPolicy(policy.Policies, "standard"),
								"reducedRedundancyReplicas": replicaForPolicy(policy.Policies, "reduced-redundancy"),
								"archiveReadyReplicas":      replicaForPolicy(policy.Policies, "archive-ready"),
							},
						),
					},
				})
				middleware.WriteJSON(w, http.StatusOK, policy)
			})

			r.Get("/storage/nodes", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "settings.read", "*") {
					return
				}
				items, err := deps.StorageTopology.ListNodes(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/storage/migration-status", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "settings.read", "*") {
					return
				}
				status, err := deps.Objects.MigrationStatus(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, status)
			})

			r.Get("/storage/migrations/history", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "settings.read", "*") {
					return
				}
				limit := 20
				if raw := req.URL.Query().Get("limit"); raw != "" {
					if parsed, err := strconv.Atoi(raw); err == nil {
						limit = parsed
					}
				}
				items, err := deps.Audit.List(req.Context(), audit.ListFilter{
					Action: "storage.migration.local-to-distributed",
					Limit:  limit,
				})
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/storage/join-tokens", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "storage.manage", "*") {
					return
				}
				var body struct {
					Description      string `json:"description"`
					IntendedName     string `json:"intendedName"`
					IntendedEndpoint string `json:"intendedEndpoint"`
					Zone             string `json:"zone"`
					ExpiresInMinutes int    `json:"expiresInMinutes"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				expiresIn := 15 * time.Minute
				if body.ExpiresInMinutes > 0 {
					expiresIn = time.Duration(body.ExpiresInMinutes) * time.Minute
				}
				item, err := deps.StorageTopology.IssueJoinToken(req.Context(), body.Description, body.IntendedName, body.IntendedEndpoint, body.Zone, expiresIn)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "storage.join-token.create",
					Resource:  item.IntendedName,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"joinTokenId": item.ID, "intendedEndpoint": item.IntendedEndpoint, "zone": item.Zone, "expiresAt": item.ExpiresAt},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Post("/storage/nodes", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "storage.manage", "*") {
					return
				}
				var body struct {
					Name          string `json:"name"`
					Endpoint      string `json:"endpoint"`
					Zone          string `json:"zone"`
					OperatorState string `json:"operatorState"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.StorageTopology.RegisterNode(req.Context(), body.Name, body.Endpoint, body.Zone, body.OperatorState)
				if err != nil {
					status := http.StatusInternalServerError
					if errors.Is(err, storage.ErrInvalidOperatorState) {
						status = http.StatusBadRequest
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "storage.node.register",
					Resource:  item.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"nodeId": item.ID, "endpoint": item.Endpoint, "operatorState": item.OperatorState},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Patch("/storage/nodes/{nodeID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "storage.manage", "*") {
					return
				}
				var body struct {
					OperatorState string `json:"operatorState"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				item, err := deps.StorageTopology.UpdateNodeOperatorState(req.Context(), chi.URLParam(req, "nodeID"), body.OperatorState)
				if err != nil {
					status := http.StatusInternalServerError
					switch {
					case errors.Is(err, storage.ErrInvalidOperatorState):
						status = http.StatusBadRequest
					case errors.Is(err, pgx.ErrNoRows):
						status = http.StatusNotFound
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "storage.node.update",
					Resource:  item.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"nodeId": item.ID, "operatorState": item.OperatorState},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/storage/nodes/{nodeID}/tls/re-pin", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "storage.manage", "*") {
					return
				}
				item, err := deps.StorageTopology.RePinNodeTLSIdentity(req.Context(), chi.URLParam(req, "nodeID"))
				if err != nil {
					status := http.StatusInternalServerError
					switch {
					case errors.Is(err, storage.ErrNodeTLSIdentityUnavailable):
						status = http.StatusBadRequest
					case errors.Is(err, pgx.ErrNoRows):
						status = http.StatusNotFound
					}
					middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "storage.node.tls.repin",
					Resource:  item.Name,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"nodeId":         item.ID,
						"endpoint":       item.Endpoint,
						"tlsIdentity":    item.Metadata["tlsIdentityStatus"],
						"tlsFingerprint": item.Metadata["tlsExpectedFingerprintSha256"],
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Post("/storage/migrations/local-to-distributed", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "storage.manage", "*") {
					return
				}
				var body struct {
					Limit int `json:"limit"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				migrated, err := deps.Objects.MigrateLocalObjectsToDistributed(req.Context(), body.Limit)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				status, statusErr := deps.Objects.MigrationStatus(req.Context())
				if statusErr != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": statusErr.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "storage.migration.local-to-distributed",
					Resource:  "storage",
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"migratedCount":       migrated,
						"pendingLocalObjects": status.PendingLocalObjects,
						"distributedObjects":  status.DistributedObjects,
					},
				})
				middleware.WriteJSON(w, http.StatusOK, map[string]any{
					"migratedCount":       migrated,
					"pendingLocalObjects": status.PendingLocalObjects,
					"distributedObjects":  status.DistributedObjects,
				})
			})

			r.Get("/storage/placements", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "settings.read", "*") {
					return
				}
				limit := 100
				if raw := req.URL.Query().Get("limit"); raw != "" {
					if parsed, err := strconv.Atoi(raw); err == nil {
						limit = parsed
					}
				}
				items, err := deps.StorageTopology.ListPlacements(req.Context(), storage.PlacementFilter{
					BucketID:  req.URL.Query().Get("bucketId"),
					Key:       req.URL.Query().Get("key"),
					VersionID: req.URL.Query().Get("versionId"),
					Limit:     limit,
				})
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/quotas", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "quota.read", "*") {
					return
				}
				buckets, err := deps.Quotas.ListBucketQuotas(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				users, err := deps.Quotas.ListUserQuotas(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{
					"bucketItems": buckets,
					"userItems":   users,
				})
			})

			r.Put("/quotas/buckets/{bucketID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "quota.manage", "*") {
					return
				}
				bucketID := chi.URLParam(req, "bucketID")
				var body struct {
					MaxBytes                *int64 `json:"maxBytes"`
					MaxObjects              *int64 `json:"maxObjects"`
					WarningThresholdPercent *int32 `json:"warningThresholdPercent"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				beforeItems, err := deps.Quotas.ListBucketQuotas(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findBucketQuota(beforeItems, bucketID)
				if err := deps.Quotas.UpdateBucketQuota(req.Context(), bucketID, body.MaxBytes, body.MaxObjects, body.WarningThresholdPercent); err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				afterItems, err := deps.Quotas.ListBucketQuotas(req.Context())
				if err == nil {
					after := findBucketQuota(afterItems, bucketID)
					_ = deps.Audit.Record(req.Context(), audit.Entry{
						Actor:     claims.Email,
						Action:    "quota.bucket.update",
						Resource:  bucketID,
						Outcome:   "success",
						RequestID: req.Header.Get("X-Request-Id"),
						Detail: map[string]any{
							"before": before,
							"after":  after,
						},
					})
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			})

			r.Put("/quotas/users/{userID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "quota.manage", "*") {
					return
				}
				userID := chi.URLParam(req, "userID")
				var body struct {
					MaxBytes                *int64 `json:"maxBytes"`
					WarningThresholdPercent *int32 `json:"warningThresholdPercent"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				beforeItems, err := deps.Quotas.ListUserQuotas(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findUserQuota(beforeItems, userID)
				if err := deps.Quotas.UpdateUserQuota(req.Context(), userID, body.MaxBytes, body.WarningThresholdPercent); err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				afterItems, err := deps.Quotas.ListUserQuotas(req.Context())
				if err == nil {
					after := findUserQuota(afterItems, userID)
					_ = deps.Audit.Record(req.Context(), audit.Entry{
						Actor:     claims.Email,
						Action:    "quota.user.update",
						Resource:  userID,
						Outcome:   "success",
						RequestID: req.Header.Get("X-Request-Id"),
						Detail: map[string]any{
							"before": before,
							"after":  after,
						},
					})
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			})

			r.Get("/roles", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "role.read", "*") {
					return
				}
				items, err := deps.Policies.ListRoles(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Get("/roles/{roleName}/statements", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "role.read", "*") {
					return
				}
				items, err := deps.Policies.ListStatementsForRole(req.Context(), chi.URLParam(req, "roleName"))
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/roles/{roleName}/statements", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				var body struct {
					Action     string            `json:"action"`
					Resource   string            `json:"resource"`
					Effect     string            `json:"effect"`
					Conditions map[string]string `json:"conditions"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Action == "" || body.Resource == "" || body.Effect == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "action, resource, and effect are required"})
					return
				}
				item, err := deps.Policies.CreateStatement(req.Context(), chi.URLParam(req, "roleName"), body.Action, body.Resource, body.Effect, body.Conditions)
				if err != nil {
					writePolicyError(w, err)
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "role.statement.create",
					Resource:  chi.URLParam(req, "roleName"),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"id":         item.ID,
						"roleName":   item.RoleName,
						"action":     item.Action,
						"resource":   item.Resource,
						"effect":     item.Effect,
						"conditions": item.Conditions,
					},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Patch("/roles/{roleName}/statements/{statementID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				var body struct {
					Action     string            `json:"action"`
					Resource   string            `json:"resource"`
					Effect     string            `json:"effect"`
					Conditions map[string]string `json:"conditions"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				beforeItems, err := deps.Policies.ListStatementsForRole(req.Context(), chi.URLParam(req, "roleName"))
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findStatement(beforeItems, chi.URLParam(req, "statementID"))
				item, err := deps.Policies.UpdateStatement(req.Context(), chi.URLParam(req, "statementID"), body.Action, body.Resource, body.Effect, body.Conditions)
				if err != nil {
					writePolicyError(w, err)
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "role.statement.update",
					Resource:  chi.URLParam(req, "roleName"),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"before": before,
						"after":  item,
					},
				})
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Delete("/roles/{roleName}/statements/{statementID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				beforeItems, err := deps.Policies.ListStatementsForRole(req.Context(), chi.URLParam(req, "roleName"))
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findStatement(beforeItems, chi.URLParam(req, "statementID"))
				if err := deps.Policies.DeleteStatement(req.Context(), chi.URLParam(req, "statementID")); err != nil {
					writePolicyError(w, err)
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "role.statement.delete",
					Resource:  chi.URLParam(req, "roleName"),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"before": before},
				})
				w.WriteHeader(http.StatusNoContent)
			})

			r.Get("/role-bindings", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "role.read", "*") {
					return
				}
				items, err := deps.Policies.ListBindings(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/role-bindings", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				var body struct {
					SubjectType string `json:"subjectType"`
					SubjectID   string `json:"subjectId"`
					Resource    string `json:"resource"`
					RoleName    string `json:"roleName"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.SubjectType == "" || body.SubjectID == "" || body.RoleName == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "subjectType, subjectId, and roleName are required"})
					return
				}
				if body.Resource == "" {
					body.Resource = "*"
				}
				item, err := deps.Policies.UpsertBinding(req.Context(), body.SubjectType, body.SubjectID, body.Resource, body.RoleName)
				if err != nil {
					writePolicyError(w, err)
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "role-binding.create",
					Resource:  body.Resource,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail: map[string]any{
						"id":          item.ID,
						"subjectType": item.SubjectType,
						"subjectId":   item.SubjectID,
						"resource":    item.Resource,
						"roleName":    item.RoleName,
					},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Post("/policy-evaluate", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "role.read", "*") {
					return
				}
				var body struct {
					SubjectType  string `json:"subjectType"`
					SubjectID    string `json:"subjectId"`
					FallbackRole string `json:"fallbackRole"`
					Action       string `json:"action"`
					Resource     string `json:"resource"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.SubjectType == "" || body.SubjectID == "" || body.FallbackRole == "" || body.Action == "" || body.Resource == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "subjectType, subjectId, fallbackRole, action, and resource are required"})
					return
				}
				item, err := deps.Policies.ExplainSubjectAuthorization(req.Context(), body.SubjectType, body.SubjectID, body.FallbackRole, body.Action, body.Resource)
				if err != nil {
					writePolicyError(w, err)
					return
				}
				middleware.WriteJSON(w, http.StatusOK, item)
			})

			r.Delete("/role-bindings/{bindingID}", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				beforeItems, err := deps.Policies.ListBindings(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findBinding(beforeItems, chi.URLParam(req, "bindingID"))
				if err := deps.Policies.DeleteBinding(req.Context(), chi.URLParam(req, "bindingID")); err != nil {
					writePolicyError(w, err)
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "role-binding.delete",
					Resource:  chi.URLParam(req, "bindingID"),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"before": before},
				})
				w.WriteHeader(http.StatusNoContent)
			})

			r.Get("/admin-tokens", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "admin-token.read", "*") {
					return
				}
				items, err := deps.AdminTokens.List(req.Context(), 100)
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Post("/admin-tokens", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "admin-token.create", "*") {
					return
				}
				claims := middleware.ClaimsFromContext(req.Context())
				var body struct {
					Role          string `json:"role"`
					Description   string `json:"description"`
					ExpiresInDays int    `json:"expiresInDays"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Role == "" {
					body.Role = claims.Role
				}
				if claims.Role != "superadmin" && body.Role != claims.Role {
					middleware.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "cannot create admin token for a different role"})
					return
				}
				if body.ExpiresInDays <= 0 {
					body.ExpiresInDays = 30
				}
				if body.ExpiresInDays > 365 {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "expiresInDays must be 365 or less"})
					return
				}
				item, err := deps.AdminTokens.Create(req.Context(), claims.UserID, body.Role, body.Description, time.Now().UTC().Add(time.Duration(body.ExpiresInDays)*24*time.Hour))
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     claims.Email,
					Action:    "admin-token.create",
					Resource:  body.Role,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"description": body.Description, "expiresInDays": body.ExpiresInDays},
				})
				middleware.WriteJSON(w, http.StatusCreated, item)
			})

			r.Get("/credentials", func(w http.ResponseWriter, req *http.Request) {
				if !authorize(w, req, deps.Authorizer, "credential.create", "*") {
					return
				}
				items, err := deps.Credentials.List(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
			})

			r.Patch("/users/{userID}/role", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				var body struct {
					Role string `json:"role"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Role == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "role is required"})
					return
				}
				beforeItems, err := deps.Users.List(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findUser(beforeItems, chi.URLParam(req, "userID"))
				if err := deps.Users.UpdateRole(req.Context(), chi.URLParam(req, "userID"), body.Role); err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				afterItems, err := deps.Users.List(req.Context())
				if err == nil {
					after := findUser(afterItems, chi.URLParam(req, "userID"))
					_ = deps.Audit.Record(req.Context(), audit.Entry{
						Actor:     claims.Email,
						Action:    "user.role.update",
						Resource:  chi.URLParam(req, "userID"),
						Outcome:   "success",
						RequestID: req.Header.Get("X-Request-Id"),
						Detail: map[string]any{
							"before": before,
							"after":  after,
						},
					})
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			})

			r.Patch("/credentials/{accessKey}/role", func(w http.ResponseWriter, req *http.Request) {
				claims := middleware.ClaimsFromContext(req.Context())
				if !authorize(w, req, deps.Authorizer, "role.manage", "*") {
					return
				}
				var body struct {
					Role string `json:"role"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
					return
				}
				if body.Role == "" {
					middleware.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "role is required"})
					return
				}
				beforeItems, err := deps.Credentials.List(req.Context())
				if err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				before := findCredential(beforeItems, chi.URLParam(req, "accessKey"))
				if err := deps.Credentials.UpdateRole(req.Context(), chi.URLParam(req, "accessKey"), body.Role); err != nil {
					middleware.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
				afterItems, err := deps.Credentials.List(req.Context())
				if err == nil {
					after := findCredential(afterItems, chi.URLParam(req, "accessKey"))
					_ = deps.Audit.Record(req.Context(), audit.Entry{
						Actor:     claims.Email,
						Action:    "credential.role.update",
						Resource:  chi.URLParam(req, "accessKey"),
						Outcome:   "success",
						RequestID: req.Header.Get("X-Request-Id"),
						Detail: map[string]any{
							"before": before,
							"after":  after,
						},
					})
				}
				middleware.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
			})
		})
	})
}

func objectPutInput(file multipart.File, header *multipart.FileHeader, key, contentType, createdBy string) (objects.PutInput, error) {
	expectedSize := header.Size
	if expectedSize == 0 {
		expectedSize = -1
	}
	return objects.PutInput{
		Key:          key,
		ContentType:  contentType,
		CreatedBy:    createdBy,
		ExpectedSize: expectedSize,
		Body:         file,
	}, nil
}

func findBucketQuota(items []quotas.BucketQuota, bucketID string) map[string]any {
	for _, item := range items {
		if item.BucketID == bucketID {
			return map[string]any{
				"bucketId":                item.BucketID,
				"bucketName":              item.BucketName,
				"maxBytes":                item.MaxBytes,
				"maxObjects":              item.MaxObjects,
				"warningThresholdPercent": item.WarningThresholdPercent,
			}
		}
	}
	return nil
}

func findUserQuota(items []quotas.UserQuota, userID string) map[string]any {
	for _, item := range items {
		if item.UserID == userID {
			return map[string]any{
				"userId":                  item.UserID,
				"email":                   item.Email,
				"maxBytes":                item.MaxBytes,
				"warningThresholdPercent": item.WarningThresholdPercent,
			}
		}
	}
	return nil
}

func findStatement(items []policies.StatementRecord, statementID string) map[string]any {
	for _, item := range items {
		if item.ID == statementID {
			return map[string]any{
				"id":         item.ID,
				"roleName":   item.RoleName,
				"action":     item.Action,
				"resource":   item.Resource,
				"effect":     item.Effect,
				"conditions": item.Conditions,
			}
		}
	}
	return nil
}

func findBinding(items []policies.SubjectBinding, bindingID string) map[string]any {
	for _, item := range items {
		if item.ID == bindingID {
			return map[string]any{
				"id":          item.ID,
				"subjectType": item.SubjectType,
				"subjectId":   item.SubjectID,
				"resource":    item.Resource,
				"roleName":    item.RoleName,
			}
		}
	}
	return nil
}

func findUser(items []map[string]any, userID string) map[string]any {
	for _, item := range items {
		if id, _ := item["id"].(string); id == userID {
			return item
		}
	}
	return nil
}

func findCredential(items []map[string]any, accessKey string) map[string]any {
	for _, item := range items {
		if key, _ := item["accessKey"].(string); key == accessKey {
			return item
		}
	}
	return nil
}

func authorize(w http.ResponseWriter, req *http.Request, authorizer *authz.Authorizer, action, resource string) bool {
	claims := middleware.ClaimsFromContext(req.Context())
	if claims == nil {
		middleware.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing claims"})
		return false
	}
	if _, err := authorizer.CheckSubject(req.Context(), claims.SubjectType, claims.SubjectID, claims.Role, action, resource); err != nil {
		status := http.StatusForbidden
		if !errors.Is(err, authz.ErrForbidden) {
			status = http.StatusInternalServerError
		}
		middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func writePolicyError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, policies.ErrInvalidRoleName),
		errors.Is(err, policies.ErrInvalidEffect),
		errors.Is(err, policies.ErrInvalidAction),
		errors.Is(err, policies.ErrInvalidResource),
		errors.Is(err, policies.ErrInvalidSubjectID),
		errors.Is(err, policies.ErrInvalidSubjectType):
		status = http.StatusBadRequest
	case errors.Is(err, policies.ErrStatementNotFound),
		errors.Is(err, policies.ErrBindingNotFound):
		status = http.StatusNotFound
	case errors.Is(err, policies.ErrProtectedRoleStatement):
		status = http.StatusConflict
	}
	middleware.WriteJSON(w, status, map[string]string{"error": err.Error()})
}

func applyQuotaWarningHeaders(w http.ResponseWriter, ctx context.Context, quotaSvc *quotas.Service, bucketID, userID string) {
	if quotaSvc == nil {
		return
	}
	warnings, err := quotaSvc.CurrentWarnings(ctx, bucketID, userID)
	if err != nil {
		return
	}
	if warnings.BucketBytes {
		w.Header().Set("X-S3P-Quota-Warning-Bucket-Bytes", "true")
	}
	if warnings.BucketCount {
		w.Header().Set("X-S3P-Quota-Warning-Bucket-Objects", "true")
	}
	if warnings.UserBytes {
		w.Header().Set("X-S3P-Quota-Warning-User-Bytes", "true")
	}
}

func replicaForPolicy(policies []config.StorageClassPolicy, name string) int {
	for _, policy := range policies {
		if policy.Name == name {
			return policy.DefaultReplicas
		}
	}
	return 0
}

func diffSettingsFields(before, after map[string]any) []string {
	changed := make([]string, 0)
	for key, beforeValue := range before {
		afterValue, ok := after[key]
		if !ok {
			changed = append(changed, key)
			continue
		}
		if fmt.Sprintf("%v", beforeValue) != fmt.Sprintf("%v", afterValue) {
			changed = append(changed, key)
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			changed = append(changed, key)
		}
	}
	return changed
}
