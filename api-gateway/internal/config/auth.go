package config

import (
	"fmt"
	"net/url"
	"strings"
)

type Auth struct {
	IssuerURL         string   `json:"issuer_url"`
	JWKSURL           string   `json:"jwks_url"`
	Audience          string   `json:"audience"`
	RequiredRoles     []string `json:"required_roles"`
	AllowInsecureHTTP bool     `json:"allow_insecure_http"`
}

func (a Auth) Validate() error {
	for name, value := range map[string]string{"issuer_url": a.IssuerURL, "jwks_url": a.JWKSURL} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(value, " \t\r\n") {
			return fmt.Errorf("%s must be an absolute URL without credentials, query or fragment", name)
		}
		if u.Scheme != "https" && (u.Scheme != "http" || !a.AllowInsecureHTTP) {
			return fmt.Errorf("%s must use https (allow_insecure_http is for local development only)", name)
		}
	}
	if strings.TrimSpace(a.Audience) == "" || a.Audience != strings.TrimSpace(a.Audience) {
		return fmt.Errorf("audience must be non-empty and have no surrounding whitespace")
	}
	if len(a.RequiredRoles) == 0 {
		return fmt.Errorf("required_roles must contain at least one API client role")
	}
	seen := make(map[string]bool)
	for _, role := range a.RequiredRoles {
		if strings.TrimSpace(role) == "" || role != strings.TrimSpace(role) || seen[role] {
			return fmt.Errorf("required_roles contains an empty, padded or duplicate role")
		}
		seen[role] = true
	}
	return nil
}
