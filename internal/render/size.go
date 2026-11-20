package render

import "fmt"

// Size is a byte count as du -h prints it: 512B, 12K, 3.4M, or 1.2G, in
// powers of 1024.
func Size(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	value, suffix := float64(n), ""
	for _, s := range []string{"K", "M", "G", "T"} {
		value /= unit
		suffix = s
		if value < unit {
			break
		}
	}
	if value < 10 {
		return fmt.Sprintf("%.1f%s", value, suffix)
	}
	return fmt.Sprintf("%.0f%s", value, suffix)
}
