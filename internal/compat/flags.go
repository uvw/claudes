package compat

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// PinsFlag is the server-side flag behind a second way Claude Code writes
// pins.json: through its storage layer, which may not take pins.json.lock.
// While it is off, Claude Code takes the lock claudes takes.
const PinsFlag = "tengu_hover_rest"

// FlagOn reads a flag from the features Claude Code caches in its settings
// file. found is false when the file or the flag is missing. Any value but
// false or null counts as on.
func FlagOn(configFile, flag string) (on, found bool, err error) {
	data, err := os.ReadFile(configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	var config struct {
		Features map[string]json.RawMessage `json:"cachedGrowthBookFeatures"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false, false, err
	}
	value, found := config.Features[flag]
	if !found {
		return false, false, nil
	}
	var off bool
	switch string(value) {
	case "false", "null":
		off = true
	}
	return !off, true, nil
}
