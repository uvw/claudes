//go:build unix

package shell

var (
	resumeCommand = posixResume
	removeCommand = posixRemove
	arg           = Quote
)
