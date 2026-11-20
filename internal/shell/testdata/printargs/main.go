// Command printargs stands in for claude in the PowerShell fuzz target: it
// prints its name and each argument in base64, so any text survives.
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	out := []string{"claude"}
	for _, a := range os.Args[1:] {
		out = append(out, base64.StdEncoding.EncodeToString([]byte(a)))
	}
	fmt.Println(strings.Join(out, " "))
}
