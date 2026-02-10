package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const MaxUserMacros = 256

func ReadResourceFile(path string, macros *[MaxUserMacros]string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open resource file %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		// Format: $USERn$=value
		if !strings.HasPrefix(line, "$USER") {
			continue
		}
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}
		varName := line[:eqIdx]
		val := line[eqIdx+1:]

		// Extract number from $USERn$
		if !strings.HasPrefix(varName, "$USER") || !strings.HasSuffix(varName, "$") {
			continue
		}
		numStr := varName[5 : len(varName)-1]
		num, err := strconv.Atoi(numStr)
		if err != nil || num < 1 || num > MaxUserMacros {
			return fmt.Errorf("%s:%d: invalid USER macro number: %s", path, lineNum, numStr)
		}
		macros[num-1] = val // 1-based to 0-based
	}
	return scanner.Err()
}
