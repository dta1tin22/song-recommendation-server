package auth

import "time"

const (
	// lockoutMaxFailedLogins is the number of failed attempts that locks an
	// account. Policy lives here, not in the repository, which only persists the
	// outcome of the threshold it is given.
	lockoutMaxFailedLogins int16 = 5

	// lockoutDuration is how long an account stays locked after reaching the
	// threshold.
	lockoutDuration = 15 * time.Minute
)
