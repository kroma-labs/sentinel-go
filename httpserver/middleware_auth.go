package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
)

// ServiceAuthConfig configures the service-to-service authentication middleware.
type ServiceAuthConfig struct {
	// Validator checks if the client_id and passkey are valid.
	Validator CredentialValidator

	// ClientIDHeader is the header name for client ID.
	// Default: "Client-ID"
	ClientIDHeader string

	// PassKeyHeader is the header name for passkey.
	// Default: "Pass-Key"
	PassKeyHeader string
}

// CredentialValidator validates service credentials.
type CredentialValidator interface {
	// Validate checks if the client_id and passkey are valid.
	// Returns nil if valid, error if invalid.
	Validate(ctx context.Context, clientID, passkey string) error
}

// CredentialValidatorFunc is an adapter to allow ordinary functions as validators.
type CredentialValidatorFunc func(ctx context.Context, clientID, passkey string) error

func (f CredentialValidatorFunc) Validate(ctx context.Context, clientID, passkey string) error {
	return f(ctx, clientID, passkey)
}

// ErrInvalidCredentials is returned when authentication fails.
// Intentionally generic to not reveal which part of credentials is wrong.
var ErrInvalidCredentials = errors.New("invalid credentials")

// MemoryCredentialValidator validates against an in-memory map.
// Useful for credentials loaded from environment variables.
type MemoryCredentialValidator struct {
	// Clients maps client_id to passkey.
	clients map[string]string
}

// NewMemoryCredentialValidator creates a validator from a map of client_id -> passkey.
//
// Example:
//
//	validator := httpserver.NewMemoryCredentialValidator(map[string]string{
//	    os.Getenv("SERVICE_CLIENT_ID"): os.Getenv("SERVICE_PASS_KEY"),
//	})
func NewMemoryCredentialValidator(clients map[string]string) *MemoryCredentialValidator {
	return &MemoryCredentialValidator{clients: clients}
}

func (v *MemoryCredentialValidator) Validate(_ context.Context, clientID, passkey string) error {
	if clientID == "" || passkey == "" {
		return ErrInvalidCredentials
	}

	expectedPasskey, exists := v.clients[clientID]
	if !exists || expectedPasskey != passkey {
		return ErrInvalidCredentials
	}
	return nil
}

// SQLCredentialValidator validates against a database table.
// Expects a table with client_id and passkey columns.
type SQLCredentialValidator struct {
	db    *sql.DB
	query string
}

// NewSQLCredentialValidator creates a validator that queries a database.
//
// The query should return the passkey for a given client_id.
// Example query: "SELECT passkey FROM service_credentials WHERE client_id = $1 AND is_active = true"
//
// Example:
//
//	validator := httpserver.NewSQLCredentialValidator(
//	    db,
//	    "SELECT passkey FROM service_credentials WHERE client_id = $1",
//	)
func NewSQLCredentialValidator(db *sql.DB, query string) *SQLCredentialValidator {
	return &SQLCredentialValidator{db: db, query: query}
}

func (v *SQLCredentialValidator) Validate(ctx context.Context, clientID, passkey string) error {
	if clientID == "" || passkey == "" {
		return ErrInvalidCredentials
	}

	var storedPasskey string
	err := v.db.QueryRowContext(ctx, v.query, clientID).Scan(&storedPasskey)
	if err != nil {
		// Don't reveal if client_id doesn't exist
		return ErrInvalidCredentials
	}

	if storedPasskey != passkey {
		return ErrInvalidCredentials
	}
	return nil
}

// ServiceAuth returns middleware that validates service-to-service credentials.
//
// Requires both Client-ID and Pass-Key headers to be present and valid.
// On failure, returns 401 Unauthorized with a generic error message.
//
// Example with in-memory validator:
//
//	validator := httpserver.NewMemoryCredentialValidator(map[string]string{
//	    os.Getenv("CLIENT_ID"): os.Getenv("PASS_KEY"),
//	})
//
//	mux.Handle("/internal/", httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	})(internalHandler))
//
// Example with database validator:
//
//	validator := httpserver.NewSQLCredentialValidator(db,
//	    "SELECT passkey FROM service_credentials WHERE client_id = $1 AND is_active = true",
//	)
//
//	mux.Handle("/internal/", httpserver.ServiceAuth(httpserver.ServiceAuthConfig{
//	    Validator: validator,
//	})(internalHandler))
func ServiceAuth(cfg ServiceAuthConfig) Middleware {
	if cfg.ClientIDHeader == "" {
		cfg.ClientIDHeader = "Client-ID"
	}
	if cfg.PassKeyHeader == "" {
		cfg.PassKeyHeader = "Pass-Key"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := r.Header.Get(cfg.ClientIDHeader)
			passkey := r.Header.Get(cfg.PassKeyHeader)

			if err := cfg.Validator.Validate(r.Context(), clientID, passkey); err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized",
					Error{Field: "auth", Message: "invalid credentials"})
				return
			}

			// Add client_id to context for downstream use
			ctx := context.WithValue(r.Context(), clientIDContextKey{}, clientID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIDContextKey is the context key for client ID.
type clientIDContextKey struct{}

// ClientIDFromContext returns the client_id from the request context.
// Returns empty string if not authenticated via ServiceAuth.
func ClientIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(clientIDContextKey{}).(string); ok {
		return v
	}
	return ""
}
