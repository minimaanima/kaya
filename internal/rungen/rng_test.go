package rungen

import "testing"

func TestSplitMix64GoldenSequence(t *testing.T) {
	rng := splitMix64{state: 0}
	want := []uint64{
		0xe220a8397b1dcdaf,
		0x6e789e6aa1b965f4,
		0x06c45d188009454f,
	}

	for i, expected := range want {
		if got := rng.next(); got != expected {
			t.Fatalf("next %d = %#x, want %#x", i, got, expected)
		}
	}
}

func TestSplitMix64BoundedStaysInsideRange(t *testing.T) {
	rng := splitMix64{state: 17}
	for i := 0; i < 10_000; i++ {
		got := rng.bounded(9)
		if got >= 9 {
			t.Fatalf("bounded(9) = %d", got)
		}
	}
}
