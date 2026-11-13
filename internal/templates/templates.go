package templates

import (
	"errors"
	"strings"
)

const MaxLength = 300

var ErrMissingCode = errors.New("template body must contain {{code}}")
var ErrTooLong = errors.New("template body must be at most 300 characters")

func Validate(body string) error {
	if !strings.Contains(body, "{{code}}") {
		return ErrMissingCode
	}
	if len(body) > MaxLength {
		return ErrTooLong
	}
	return nil
}
