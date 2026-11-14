package main

import "testing"

func TestMigrationDatabaseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "postgres scheme",
			url:  "postgres://user:pass@localhost:5432/dartotp?sslmode=require",
			want: "pgx5://user:pass@localhost:5432/dartotp?sslmode=require",
		},
		{
			name: "postgresql scheme",
			url:  "postgresql://user:pass@localhost:5432/dartotp?sslmode=require",
			want: "pgx5://user:pass@localhost:5432/dartotp?sslmode=require",
		},
		{
			name: "already normalized scheme",
			url:  "pgx5://user:pass@localhost:5432/dartotp?sslmode=require",
			want: "pgx5://user:pass@localhost:5432/dartotp?sslmode=require",
		},
		{
			name: "other scheme unchanged",
			url:  "mysql://user:pass@localhost:3306/dartotp",
			want: "mysql://user:pass@localhost:3306/dartotp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := migrationDatabaseURL(tt.url); got != tt.want {
				t.Errorf("migrationDatabaseURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
