//go:build !windows && !darwin

package game

import "errors"

func CheckGameLaunch(bool) error {
	return errors.New("launch the game on its native operating system")
}
