package bot

import (
	"errors"

	"github.com/daddymaou/dart-otp/internal/phone"
)

var ErrOwnNumber = errors.New("Please share your own number using the button.")
var ErrTokenExpired = errors.New("This link has expired. Please request a new code from the app.")
var ErrPhoneMismatch = errors.New("That number does not match the one the app asked for.")

// ValidateContact is the pure logic of accepting a shared contact.
// It returns the normalized phone if all checks pass.
func ValidateContact(senderID, contactUserID int64, contactPhone, pendingTokenHash string) (string, error) {
	if contactUserID != senderID {
		return "", ErrOwnNumber
	}
	if pendingTokenHash == "" {
		return "", ErrTokenExpired
	}
	p, err := phone.Normalize(contactPhone)
	if err != nil {
		return "", ErrOwnNumber
	}
	return p, nil
}

// CheckPhoneMatch compares the normalized contact phone against the token's phone.
func CheckPhoneMatch(contactPhone, tokenPhone string) error {
	if contactPhone != tokenPhone {
		return ErrPhoneMismatch
	}
	return nil
}
