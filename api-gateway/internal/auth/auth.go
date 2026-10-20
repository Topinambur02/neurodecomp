package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topinambur02/apigateway/internal/config"
)

type principalKey struct{}

type Principal struct {
	Subject string
	Issuer  string
	Roles   []string
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

type Authenticator struct {
	verifier *oidc.IDTokenVerifier
	audience string
	roles    []string
}

func New(ctx context.Context, cfg config.Auth) (*Authenticator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	keySet := oidc.NewRemoteKeySet(oidc.ClientContext(ctx, client), cfg.JWKSURL)
	verifier := oidc.NewVerifier(cfg.IssuerURL, keySet, &oidc.Config{
		ClientID: cfg.Audience, SupportedSigningAlgs: []string{oidc.RS256},
	})
	return &Authenticator{verifier: verifier, audience: cfg.Audience, roles: append([]string(nil), cfg.RequiredRoles...)}, nil
}

func (a *Authenticator) Require(next http.Handler, roles ...string) http.Handler {
	if len(roles) == 0 {
		roles = a.roles
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := r.Header.Values("Authorization")
		if len(headers) != 1 || len(headers[0]) > 16384 {
			reject(w, http.StatusUnauthorized)
			return
		}
		parts := strings.Fields(headers[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			reject(w, http.StatusUnauthorized)
			return
		}
		token, err := a.verifier.Verify(r.Context(), parts[1])
		if err != nil {
			reject(w, http.StatusUnauthorized)
			return
		}
		var claims struct {
			Type           string `json:"typ"`
			NotBefore      int64  `json:"nbf"`
			ResourceAccess map[string]struct {
				Roles []string `json:"roles"`
			} `json:"resource_access"`
		}
		if err := token.Claims(&claims); err != nil || claims.Type != "Bearer" || token.Subject == "" || claims.NotBefore > time.Now().Unix() {
			reject(w, http.StatusUnauthorized)
			return
		}
		clientRoles := claims.ResourceAccess[a.audience].Roles
		allowed := false
		for _, required := range roles {
			for _, role := range clientRoles {
				if role == required {
					allowed = true
				}
			}
		}
		if !allowed {
			reject(w, http.StatusForbidden)
			return
		}
		principal := Principal{Subject: token.Subject, Issuer: token.Issuer, Roles: clientRoles}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
	})
}

func reject(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="neurodecomp"`)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": http.StatusText(status)})
}
