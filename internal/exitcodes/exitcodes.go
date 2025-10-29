package exitcodes

const (
	ExitCodeSuccess = iota
	ExitCodeEnvFileError
	ExitCodeConfigError
	ExitCodeLoggerError
	ExitCodeRunError
)
