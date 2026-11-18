package render

import "testing"

func TestSizeReadsAsDuDoes(t *testing.T) {
	for n, want := range map[int64]string{
		0:             "0B",
		512:           "512B",
		1024:          "1.0K",
		12 * 1024:     "12K",
		3_500_000:     "3.3M",
		1_300_000_000: "1.2G",
		5 << 40:       "5.0T",
	} {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}
