package utils

import "flag"

var isDev bool
var isDevShort bool

func SetupFlags() {
	flag.BoolVar(&isDev, "dev", false, "Running in development environment")
	flag.BoolVar(&isDevShort, "D", false, "Running in development environment")
	flag.Parse()

	Dev(func() { println("Dev!") })
}

func Dev(fn func()) {
	if isDev || isDevShort {
		checkFn(fn)
	}
}

func Prod(fn func()) {
	if !(isDev || isDevShort) {
		checkFn(fn)
	}
}

func checkFn(fn func()) {
	if fn != nil {
		fn()
	}
}
