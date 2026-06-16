package cmd

import (
	"errors"

	"github.com/steipete/gogcli/internal/sheetsvalues"
)

func sheetsValuesPlannerError(err error) error {
	var validationErr sheetsvalues.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}

	return err
}
