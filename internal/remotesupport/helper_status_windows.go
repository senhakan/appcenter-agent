//go:build windows

package remotesupport

import (
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
)

func helperProcessStatus() (bool, int) {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+vncHelperName, "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false, 0
	}
	line := strings.TrimSpace(string(out))
	if line == "" || strings.EqualFold(line, "INFO: No tasks are running which match the specified criteria.") {
		return false, 0
	}
	r := csv.NewReader(strings.NewReader(line))
	rec, err := r.Read()
	if err != nil || len(rec) < 2 {
		return true, 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(rec[1]))
	return true, pid
}
