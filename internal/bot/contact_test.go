package bot

import "testing"

func TestValidateContact(t *testing.T) {
	if _, err := ValidateContact(1, 2, "+2348012345678", "hash"); err != ErrOwnNumber {
		t.Errorf("someone else's contact: got %v", err)
	}
	if _, err := ValidateContact(1, 1, "+2348012345678", ""); err != ErrTokenExpired {
		t.Errorf("missing token: got %v", err)
	}
	p, err := ValidateContact(1, 1, "2348012345678", "hash")
	if err != nil {
		t.Fatalf("valid contact: %v", err)
	}
	if p != "+2348012345678" {
		t.Errorf("phone = %q", p)
	}
	if err := CheckPhoneMatch("+2348012345678", "+2348099999999"); err != ErrPhoneMismatch {
		t.Errorf("mismatch: got %v", err)
	}
	if err := CheckPhoneMatch("+2348012345678", "+2348012345678"); err != nil {
		t.Errorf("match: got %v", err)
	}
}

func TestHashTokenAndPending(t *testing.T) {
	raw := NewLinkTokenRaw()
	h := HashToken(raw)
	if len(h) != 64 {
		t.Errorf("hash length = %d", len(h))
	}
	pl := NewPendingLinks()
	pl.Set(42, h, 15e9)
	if got, ok := pl.Get(42); !ok || got != h {
		t.Errorf("Get = %q, %v", got, ok)
	}
	pl.Delete(42)
	if _, ok := pl.Get(42); ok {
		t.Error("deleted entry still present")
	}
}

func TestRenderBody(t *testing.T) {
	got := RenderBody("Your <b>code</b> is {{code}}", "123456")
	want := "Your &lt;b&gt;code&lt;/b&gt; is <code>123456</code>"
	if got != want {
		t.Errorf("RenderBody = %q, want %q", got, want)
	}
}
