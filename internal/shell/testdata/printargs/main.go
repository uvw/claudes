// Command printargs stands in for claude in the PowerShell fuzz target: it
// prints its name and each argument in base64, so any text survives. When
// CLAUDE_CONFIG_DIR is set, a line before that gives its value the same way.
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	if dir, ok := os.LookupEnv("CLAUDE_CONFIG_DIR"); ok {
		fmt.Println("config " + base64.StdEncoding.EncodeToString([]byte(dir)))
	}
	out := []string{"claude"}
	for _, a := range os.Args[1:] {
		out = append(out, base64.StdEncoding.EncodeToString([]byte(a)))
	}
	fmt.Println(strings.Join(out, " "))
}
