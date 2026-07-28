package algorithm

import "time"


type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}
