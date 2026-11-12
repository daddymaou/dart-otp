package phone

import (
	"fmt"

	"github.com/nyaruka/phonenumbers"
)

func Normalize(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty phone")
	}
	if input[0] != '+' {
		input = "+" + input
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
