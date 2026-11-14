package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Plan struct{ Monthly int }

type Config struct {
	AppEnv              string
	HTTPAddr            string
	DatabaseURL         string
	JWTSecret           string
	OTPPepper           string
	CookieSecure        bool
	CORSAllowedOrigins  []string
	TelegramBotToken    string
	TelegramBotUsername string

	OTPLength        int
	OTPTTL           time.Duration
	OTPMaxAttempts   int
	OTPCooldown      time.Duration
	LinkTokenTTL     time.Duration
	TenantRatePerMin int
	AuthRatePerMin   int
	RetentionDays    int
	Plans            map[string]Plan
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getSeconds(key string, def int) time.Duration {
	return time.Duration(getInt(key, def)) * time.Second
}

func splitCSV(v string) []string {
	var out []string
	for _, part := range split(v, ",") {
		out = append(out, part)
	}
	return out
}

func split(s, sep string) []string {
	var out []string
	start := 0
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			out = append(out, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	out = append(out, s[start:])
	return out
}

func Load() (*Config, error) {
	c := &Config{
		AppEnv:              get("APP_ENV", "development"),
		HTTPAddr:            get("HTTP_ADDR", ":8080"),
		DatabaseURL:         get("DATABASE_URL", ""),
		JWTSecret:           get("JWT_SECRET", ""),
		OTPPepper:           get("OTP_PEPPER", ""),
		CookieSecure:        get("COOKIE_SECURE", "false") == "true",
		TelegramBotToken:    get("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername: get("TELEGRAM_BOT_USERNAME", "DartOTPBot"),

		OTPLength:        getInt("OTP_LENGTH", 6),
		OTPTTL:           getSeconds("OTP_TTL_SECONDS", 300),
		OTPMaxAttempts:   getInt("OTP_MAX_ATTEMPTS", 5),
		OTPCooldown:      getSeconds("OTP_COOLDOWN_SECONDS", 60),
		LinkTokenTTL:     getSeconds("LINK_TOKEN_TTL_SECONDS", 900),
		TenantRatePerMin: getInt("TENANT_RATE_PER_MINUTE", 30),
		AuthRatePerMin:   getInt("AUTH_RATE_PER_MINUTE", 10),
		RetentionDays:    getInt("RETENTION_DAYS", 30),
		Plans: map[string]Plan{
			"free": {Monthly: getInt("PLAN_FREE_MONTHLY", 100)},
			"pro":  {Monthly: getInt("PLAN_PRO_MONTHLY", 10000)},
		},
	}
	c.CORSAllowedOrigins = splitCSV(get("CORS_ALLOWED_ORIGINS", "http://localhost:3000"))

	if c.AppEnv != "test" {
		for k, v := range map[string]string{
			"DATABASE_URL":       c.DatabaseURL,
			"JWT_SECRET":         c.JWTSecret,
			"OTP_PEPPER":         c.OTPPepper,
			"TELEGRAM_BOT_TOKEN": c.TelegramBotToken,
		} {
			if v == "" {
				return nil, fmt.Errorf("missing required env var: %s", k)
			}
		}
	}
	if c.OTPLength < 4 || c.OTPLength > 10 {
		return nil, fmt.Errorf("OTP_LENGTH must be between 4 and 10")
	}
	return c, nil
}
