package setup

// ExitPaused marks a resumable setup operation that did not complete.
const ExitPaused = 2

// StageProgress reports completed stages independently of within-stage progress.
func StageProgress(total int, emit func(Event)) func() {
	done := 0
	emit(Measurement("overall", 0, done, total))
	return func() {
		done++
		emit(Measurement("overall", 0, done, total))
	}
}
