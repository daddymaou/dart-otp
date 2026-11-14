package phone

import (
	"fmt"

	"github.com/nyaruka/phonenumbers"
)

func Normalize(input string) (string, error) {
	if input == "" || input[0] != '+' {
		return "", fmt.Errorf("invalid phone")
	}
	num, err := phonenumbers.Parse(input, "")
	if err != nil {
		return "", fmt.Errorf("invalid phone")
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", fmt.Errorf("invalid phone")
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// NormalizeContact accepts Telegram contact numbers without a leading '+'.
func NormalizeContact(input string) (string, error) {
	if input != "" && input[0] != '+' {
		input = "+" + input
	}
	return Normalize(input)
}
