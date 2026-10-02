package models

type Status string

const (
	StatusPending         Status = "PENDING"
	StatusOpen            Status = "OPEN"
	StatusPartiallyFilled Status = "PARTIALLY_FILLED"
	StatusFilled          Status = "FILLED"
	StatusCancelled       Status = "CANCELLED"
	StatusRejected        Status = "REJECTED"
)

func IsValidStatus(status Status) bool {
	switch status {
	case StatusPending, StatusOpen, StatusPartiallyFilled, StatusFilled, StatusCancelled, StatusRejected:
		return true
	default:
		return false
	}
}

// A final order never changes again. orderdb's statements apply the same rule
// in their WHERE.
func (s Status) IsFinal() bool {
	return s == StatusFilled || s == StatusCancelled || s == StatusRejected
}
