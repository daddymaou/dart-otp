package config

import (
	"reflect"
	"testing"
)

func TestLoadNormalizesAdminEmails(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("ADMIN_EMAILS", " Admin@Example.com,admin@example.com, second@example.com ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := []string{"admin@example.com", "second@example.com"}
	if !reflect.DeepEqual(cfg.AdminEmails, want) {
		t.Fatalf("AdminEmails = %#v, want %#v", cfg.AdminEmails, want)
	}
}

func TestLoadRejectsInvalidAdminEmail(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("ADMIN_EMAILS", "Admin <admin@example.com>")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a display-name email entry")
	}
}

func TestLoadPlanMonthlyDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("PLAN_FREE_MONTHLY", "")
	t.Setenv("PLAN_STARTER_MONTHLY", "")
	t.Setenv("PLAN_GROWTH_MONTHLY", "")
	t.Setenv("PLAN_PRO_MONTHLY", "")
	t.Setenv("PLAN_BUSINESS_MONTHLY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	want := map[string]int{
		"free": 100, "starter": 1000, "growth": 5000, "pro": 10000, "business": 50000,
	}
	for plan, monthly := range want {
		if got := cfg.Plans[plan].Monthly; got != monthly {
			t.Errorf("%s monthly quota = %d, want %d", plan, got, monthly)
		}
	}
}
