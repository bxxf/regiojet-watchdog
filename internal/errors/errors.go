package errors

import "fmt"

const (
	NotifyServiceOk              = 0
	NotifyServiceJsonError       = 1
	NotifyServiceHttpError       = 2
	NotifyServiceHttpStatusError = 3
)

type NotifyServiceStatusError struct {
	Status int
}

func (e *NotifyServiceStatusError) Error() string {
	return fmt.Sprintf("status: %d", e.Status)
}
