package auth

import "testing"

func TestPasswordHashAndJWT(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordTooShort {
		t.Errorf("short password: got %v", err)
	}
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !CheckPassword(hash, "correct-horse-battery") {
		t.Error("password should verify")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("wrong password should not verify")
	}

	token, err := NewJWT("secret", "tenant-1")
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	id, err := ParseJWT("secret", token)
	if err != nil || id != "tenant-1" {
		t.Fatalf("parse: %q %v", id, err)
	}
	if _, err := ParseJWT("wrong-secret", token); err == nil {
		t.Error("wrong secret should fail")
	}
}
