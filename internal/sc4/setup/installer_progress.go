package setup

// ExitPaused marks a resumable setup operation that did not complete.
const ExitPaused = 2

// setupStageProgress reports completed stages, independently of any progress
// within a stage. Call the returned function after each stage succeeds.
func SetupStageProgress(total int, report func(done, total int)) func() {
	done := 0
	report(done, total)
	return func() {
		done++
		report(done, total)
	}
}
