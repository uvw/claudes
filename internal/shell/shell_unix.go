//go:build unix

package shell

var (
	resumeCommand = posixResume
	configPrefix  = posixConfig
	removeCommand = posixRemove
	arg           = Quote
)
