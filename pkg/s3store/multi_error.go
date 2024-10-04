package s3store

import (
	"errors"
)

// TODO: Replace with errors.Join
func newMultiError(errs []error) error {
	message := "Multiple errors occurred:\n"
	for _, err := range errs {
		message += "\t" + err.Error() + "\n"
	}
	return errors.New(message)
}
