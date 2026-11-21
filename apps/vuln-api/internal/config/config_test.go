package config

import (
	"net/netip"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func full() map[string]string {
	return map[string]string{
		"DATABASE_URL":                 "postgres://app@db/vulnman",
		"PUBLIC_BASE_URL":              "https://cerberoast.site",
		"OIDC_ISSUER":                  "https://auth.cerberoast.site/realms/vulnman",
		"OIDC_CLIENT_ID":               "vuln-api",
		"OIDC_CLIENT_SECRET":           "s",
		"KEYCLOAK_ADMIN_BASE_URL":      "http://keycloak-public-http.keycloak-public.svc",
		"KEYCLOAK_REALM":               "vulnman",
		"KEYCLOAK_ADMIN_CLIENT_ID":     "vuln-api-admin",
		"KEYCLOAK_ADMIN_CLIENT_SECRET": "s2",
		"VAULT_ADDR":                   "https://vault.vault.svc:8200",
		"VAULT_ROLE":                   "vuln-api-dev",
		"TRANSIT_DATA_KEY":             "vulnman-dev-data",
		"TRANSIT_REPORT_KEY":           "vulnman-dev-report",
	}
}

func TestLoadMissingRequired(t *testing.T) {
	_, err := Load(env(map[string]string{}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, k := range []string{"DATABASE_URL", "PUBLIC_BASE_URL", "OIDC_ISSUER", "OIDC_CLIENT_ID", "OIDC_CLIENT_SECRET",
		"KEYCLOAK_ADMIN_BASE_URL", "KEYCLOAK_REALM", "KEYCLOAK_ADMIN_CLIENT_ID", "KEYCLOAK_ADMIN_CLIENT_SECRET",
		"VAULT_ADDR", "VAULT_ROLE", "TRANSIT_DATA_KEY", "TRANSIT_REPORT_KEY"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err, k)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(full()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.VaultTokenPath != "/var/run/secrets/vault/token" || c.VaultCACert != "/etc/cerberus/ca.crt" {
		t.Errorf("defaults wrong: %+v", c)
	}
	if c.OIDCDiscoveryURL != c.OIDCIssuer {
		t.Errorf("discovery should default to issuer, got %q", c.OIDCDiscoveryURL)
	}
}

func TestLoadTrustedCIDRs(t *testing.T) {
	m := full()
	m["TRUSTED_PROXY_CIDRS"] = "10.42.0.0/16, 10.0.2.0/24"
	c, err := Load(env(m))
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16"), netip.MustParsePrefix("10.0.2.0/24")}
	if len(c.TrustedProxyCIDRs) != 2 || c.TrustedProxyCIDRs[0] != want[0] || c.TrustedProxyCIDRs[1] != want[1] {
		t.Errorf("got %v", c.TrustedProxyCIDRs)
	}
	m["TRUSTED_PROXY_CIDRS"] = "not-a-cidr"
	if _, err := Load(env(m)); err == nil {
		t.Error("bad CIDR accepted")
	}
}

func TestLoadMigrateOnlyNeedsMigrateURL(t *testing.T) {
	c, err := LoadMigrate(env(map[string]string{"MIGRATE_DATABASE_URL": "postgres://owner@db/vulnman"}))
	if err != nil || c != "postgres://owner@db/vulnman" {
		t.Fatalf("got %q, %v", c, err)
	}
	if _, err := LoadMigrate(env(nil)); err == nil || !strings.Contains(err.Error(), "MIGRATE_DATABASE_URL") {
		t.Fatalf("expected missing MIGRATE_DATABASE_URL, got %v", err)
	}
}

func TestLoadScannerToken(t *testing.T) {
	m := full()
	if c, err := Load(env(m)); err != nil || c.ScannerToken != "" {
		t.Fatalf("no SCANNER_TOKEN: %q %v (the scan endpoint must stay off)", c.ScannerToken, err)
	}
	m["SCANNER_TOKEN"] = "too-short"
	if _, err := Load(env(m)); err == nil {
		t.Fatal("a short SCANNER_TOKEN was accepted")
	}
	m["SCANNER_TOKEN"] = "0123456789abcdef0123456789abcdef"
	if c, err := Load(env(m)); err != nil || c.ScannerToken != m["SCANNER_TOKEN"] {
		t.Fatalf("valid SCANNER_TOKEN: %v", err)
	}
}
