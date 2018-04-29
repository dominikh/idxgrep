package log

import (
	stdlog "log"
	"os"
)

var logger = stdlog.New(os.Stderr, "", 0)

func Fatal(code int, v ...interface{}) {
	Print(v...)
	os.Exit(code)
}

func Fatalf(code int, format string, v ...interface{}) {
	Printf(format, v...)
	os.Exit(code)
}

func Fatalln(code int, v ...interface{}) {
	Println(v...)
	os.Exit(code)
}

func Print(v ...interface{}) {
	logger.Print(v...)
}

func Printf(format string, v ...interface{}) {
	logger.Printf(format, v...)
}

func Println(v ...interface{}) {
	logger.Println(v...)
}
