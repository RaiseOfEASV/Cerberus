// Package config reads vuln-api's configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// Config is everything the server needs at start-up. Secrets arrive as
// environment variables from Kubernetes Secrets (ESO), never from files in
// the image.
type Config struct {
	Port                      string
	DatabaseURL               string
	PublicBaseURL             string // e.g. https://cerberoast.site - used for OIDC redirects and Origin checks
	OIDCIssuer                string // public issuer, checked against the ID token
	OIDCDiscoveryURL          string // in-cluster URL for discovery/back-channel; defaults to OIDCIssuer
	OIDCClientID              string
	OIDCClientSecret          string
	KeycloakAdminBaseURL      string // e.g. http://keycloak-public-http.keycloak-public.svc
	KeycloakRealm             string
	KeycloakAdminClientID     string
	KeycloakAdminClientSecret string
	VaultAddr                 string
	VaultCACert               string
	VaultRole                 string
	VaultTokenPath            string
	TransitDataKey            string
	TransitReportKey          string
	TrustedProxyCIDRs         []netip.Prefix
	// ScannerToken enables POST /internal/scans/trivy for the in-cluster
	// nightly scanner (vuln-scanner). Empty = the endpoint doesn't exist.
	ScannerToken string
}

var required = []string{
	"DATABASE_URL", "PUBLIC_BASE_URL", "OIDC_ISSUER", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET",
	"KEYCLOAK_ADMIN_BASE_URL", "KEYCLOAK_REALM", "KEYCLOAK_ADMIN_CLIENT_ID", "KEYCLOAK_ADMIN_CLIENT_SECRET",
	"VAULT_ADDR", "VAULT_ROLE", "TRANSIT_DATA_KEY", "TRANSIT_REPORT_KEY",
}

// Load builds a Config. It reports every missing required variable at once.
func Load(getenv func(string) string) (Config, error) {
	var missing []string
	for _, k := range required {
		if strings.TrimSpace(getenv(k)) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	or := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Port:                      or("PORT", "8080"),
		DatabaseURL:               getenv("DATABASE_URL"),
		PublicBaseURL:             strings.TrimRight(getenv("PUBLIC_BASE_URL"), "/"),
		OIDCIssuer:                getenv("OIDC_ISSUER"),
		OIDCDiscoveryURL:          or("OIDC_DISCOVERY_URL", getenv("OIDC_ISSUER")),
		OIDCClientID:              getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:          getenv("OIDC_CLIENT_SECRET"),
		KeycloakAdminBaseURL:      strings.TrimRight(getenv("KEYCLOAK_ADMIN_BASE_URL"), "/"),
		KeycloakRealm:             getenv("KEYCLOAK_REALM"),
		KeycloakAdminClientID:     getenv("KEYCLOAK_ADMIN_CLIENT_ID"),
		KeycloakAdminClientSecret: getenv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		VaultAddr:                 strings.TrimRight(getenv("VAULT_ADDR"), "/"),
		VaultCACert:               or("VAULT_CACERT", "/etc/cerberus/ca.crt"),
		VaultRole:                 getenv("VAULT_ROLE"),
		VaultTokenPath:            or("VAULT_TOKEN_PATH", "/var/run/secrets/vault/token"),
		TransitDataKey:            getenv("TRANSIT_DATA_KEY"),
		TransitReportKey:          getenv("TRANSIT_REPORT_KEY"),
		ScannerToken:              strings.TrimSpace(getenv("SCANNER_TOKEN")),
	}
	if c.ScannerToken != "" && len(c.ScannerToken) < 32 {
		return Config{}, errors.New("SCANNER_TOKEN must be at least 32 characters")
	}
	for _, s := range strings.Split(getenv("TRUSTED_PROXY_CIDRS"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_CIDRS: %w", err)
		}
		c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, p)
	}
	return c, nil
}

// LoadMigrate returns the owner connection string for `vuln-api migrate`,
// which runs in an initContainer that only receives the owner secret.
func LoadMigrate(getenv func(string) string) (string, error) {
	v := strings.TrimSpace(getenv("MIGRATE_DATABASE_URL"))
	if v == "" {
		return "", errors.New("missing required environment variable: MIGRATE_DATABASE_URL")
	}
	return v, nil
}
