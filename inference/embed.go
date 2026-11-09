// Package inference embeds the HAT worker for distribution with the Windows EXE.
package inference

import _ "embed"

// WorkerSource is passed to the Python interpreter in WSL.
//
//go:embed worker.py
var WorkerSource string
