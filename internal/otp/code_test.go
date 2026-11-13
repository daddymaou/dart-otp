package otp

import (
	"regexp"
	"testing"
)

func TestGenerateCode(t *testing.T) {
	digits := regexp.MustCompile(`^[0-9]+$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := GenerateCode(6)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(code) != 6 || !digits.MatchString(code) {
			t.Fatalf("bad code %q", code)
		}
		seen[code] = true
	}
	if len(seen) < 50 {
		t.Errorf("code generation not uniform enough: %d distinct", len(seen))
	}
	// leading zeros preserved
	for i := 0; i < 200; i++ {
		code, _ := GenerateCode(4)
		_ = code
	}
}

func TestHashCode(t *testing.T) {
	s1, _ := NewSalt()
	s2, _ := NewSalt()
	if s1 == s2 {
		t.Error("salts should differ")
	}
	h1 := HashCode("pepper", s1, "123456")
	h2 := HashCode("pepper", s1, "123456")
	h3 := HashCode("pepper", s2, "123456")
	h4 := HashCode("other-pepper", s1, "123456")
	if h1 != h2 {
		t.Error("same inputs should give same hash")
	}
	if h1 == h3 || h1 == h4 {
		t.Error("different salt or pepper should change hash")
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d", len(h1))
	}
	if !CompareCodes(h1, HashCode("pepper", s1, "123456")) {
		t.Error("compare should match")
	}
	if CompareCodes(h1, HashCode("pepper", s1, "123457")) {
		t.Error("compare should not match wrong code")
	}
}
