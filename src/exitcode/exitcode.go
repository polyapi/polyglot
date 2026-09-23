// Package exitcode holds process exit codes from RFC §5.5.8.
package exitcode

const (
	OK             = 0
	Failure        = 1
	Usage          = 2
	Auth           = 3
	Network        = 4
	AdapterMissing = 10
	Protocol       = 11
	Partial        = 12
)
