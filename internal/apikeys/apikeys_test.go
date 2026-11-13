package apikeys

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	base62 := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	full, prefix, hash, err := Generate(false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(full, "dart_live_") || len(full) != len("dart_live_")+32 {
		t.Errorf("live key = %q", full)
	}
	if prefix != full[:12] {
		t.Errorf("prefix = %q", prefix)
	}
	if len(hash) != 64 {
		t.Errorf("hash = %q", hash)
	}
	if !base62.MatchString(strings.TrimPrefix(full, "dart_live_")) {
		t.Errorf("not base62: %q", full)
	}

	testFull, _, _, err := Generate(true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(testFull, "dart_test_") {
		t.Errorf("test key = %q", testFull)
	}
	if HashKey(full) != hash {
		t.Error("HashKey must match Generate hash")
	}
}
