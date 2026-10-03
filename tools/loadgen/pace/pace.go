// Package pace says how many requests a load run should have sent at each
// moment, so the sender keeps a fixed rate whatever each request takes: at
// any instant it sends what is due and has not been sent yet.
package pace

import "time"

type Schedule interface {
	// Requests per second at that moment of the run.
	GetRate(elapsed time.Duration) int
	// Requests the run should have sent from its start until elapsed.
	CountDue(elapsed time.Duration) int
}

type constant struct {
	rate int
}

func NewConstant(rate int) Schedule {
	return constant{rate: rate}
}

func (s constant) GetRate(time.Duration) int {
	return s.rate
}

func (s constant) CountDue(elapsed time.Duration) int {
	return int(float64(s.rate) * elapsed.Seconds())
}

// The base rate for the first third of the run, the peak for the second and
// the base again for the last: profile E.
type burst struct {
	base     int
	peak     int
	duration time.Duration
}

func NewBurst(base, peak int, duration time.Duration) Schedule {
	return burst{base: base, peak: peak, duration: duration}
}

func (s burst) GetRate(elapsed time.Duration) int {
	if elapsed >= s.duration/3 && elapsed < 2*s.duration/3 {
		return s.peak
	}

	return s.base
}

func (s burst) CountDue(elapsed time.Duration) int {
	peakStart, peakEnd := s.duration/3, 2*s.duration/3

	before := min(elapsed, peakStart)
	during := max(min(elapsed, peakEnd)-peakStart, 0)
	after := max(elapsed-peakEnd, 0)

	return int(float64(s.base)*before.Seconds() + float64(s.peak)*during.Seconds() + float64(s.base)*after.Seconds())
}
