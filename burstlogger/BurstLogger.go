package burstlogger

import "fmt"
import "os"

type BurstLogger struct {
	logs  []string
	DoLog func(string)
}

func (bl *BurstLogger) Logln(a ...any) {
	bl.logs = append(bl.logs, fmt.Sprintln(a...))
}
func (bl *BurstLogger) Logfn(str string, strs ...any) {
	bl.logs = append(bl.logs, fmt.Sprintf(str, strs...)+"\n")
}
func (bl *BurstLogger) Fire() {
	for _, message := range bl.logs {
		if bl.DoLog != nil {
			bl.DoLog(message)
		} else {
			// Default behaviour: logs should go to stderr
			os.Stderr.WriteString(message)
		}
	}
	bl.Forget()
}
func (bl *BurstLogger) Forget() {
	bl.logs = nil
}
