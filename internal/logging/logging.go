package logging

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	debugMu   sync.Mutex
	debugMode bool
)

func InitLogger() {
	if logl, ok := os.LookupEnv("FLUFFY_LOG_LEVEL"); ok {
		switch strings.ToLower(logl) {
		case "debug", "dbg":
			debugMode = true
		}
	}
}

func SetDebug(b bool) {
	debugMu.Lock()
	defer debugMu.Unlock()
	debugMode = b
}

func Debug(msg ...any) {
	debugMu.Lock()
	d := debugMode
	debugMu.Unlock()
	if d {
		fmt.Println(msg...)
	}
}

func Info(msg ...any) {
	fmt.Println(msg...)
}

func Warn(msg ...any) {
	fmt.Println(append([]any{"level=warn"}, msg...)...)
}

func Error(msg ...any) {
	fmt.Println(append([]any{"level=Error"}, msg...)...)
}
