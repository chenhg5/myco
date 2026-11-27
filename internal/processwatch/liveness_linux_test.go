package processwatch

import (
	"os"
	"strconv"
	"strings"
)

func running(pid int) bool {
	b, e := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if e != nil {
		return false
	}
	_, s, _ := strings.Cut(string(b), ") ")
	return !strings.HasPrefix(s, "Z ")
}
