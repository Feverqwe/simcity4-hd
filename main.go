package main

import (
	"os"

	"github.com/Feverqwe/simcity4-hd/internal/sc4"
)

var version = "dev"

func main() {
	os.Exit(sc4.Run(os.Args[1:], version))
}
