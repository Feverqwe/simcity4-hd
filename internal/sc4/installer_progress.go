package sc4

// setupStageProgress reports completed stages, independently of any progress
// within a stage. Call the returned function after each stage succeeds.
func setupStageProgress(total int, report func(done, total int)) func() {
	done := 0
	report(done, total)
	return func() {
		done++
		report(done, total)
	}
}
