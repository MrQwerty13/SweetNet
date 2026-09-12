package config

import "testing"

func TestProductionRequiresHTTPSOrigin(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/sweetnet")
	t.Setenv("APP_ENV", "production")
	for _, value := range []string{"http://localhost:8080", "https://example.com/", "https://example.com/path", "https://user:pass@example.com", "https://example.com?query=1"} {
		t.Setenv("APP_ORIGIN", value)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted invalid production origin %q", value)
		}
	}
	t.Setenv("APP_ORIGIN", "https://example.com")
	c, err := Load()
	if err != nil || !c.Production {
		t.Fatal("valid HTTPS configuration rejected", err)
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("APP_ORIGIN", "http://localhost:8080")
	if c, err := Load(); err != nil || c.Production {
		t.Fatal("explicit local development rejected")
	}
}
