package api

import (
	"fmt"
	"math/rand"
	"testing"
)

func BenchmarkPrevEventIDs(b *testing.B) {
	for _, x := range []int64{1, 2, 4, 10, 50, 100, 300, 500, 800, 1000, 2000, 3000, 5000, 10000} {
		benchPrevEventIDs(b, int(x))
	}
}

func benchPrevEventIDs(b *testing.B, count int) {
	b.Run(fmt.Sprintf("Benchmark%d", count), func(b *testing.B) {
		bwExtrems := generateBackwardsExtremities(b, count)
		backfiller := PerformBackfillRequest{
			BackwardsExtremities: bwExtrems,
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prevIDs := backfiller.PrevEventIDs()
			_ = prevIDs
		}
	})
}

const randomIDCharsCount = 10

func generateBackwardsExtremities(b *testing.B, count int) map[string][]string {
	b.Helper()
	result := make(map[string][]string, count)
	for i := 0; i < count; i++ {
		eventID := randomEventId(int64(i), randomIDCharsCount)
		result[eventID] = []string{
			randomEventId(int64(i+1), randomIDCharsCount),
			randomEventId(int64(i+2), randomIDCharsCount),
			randomEventId(int64(i+3), randomIDCharsCount),
		}
	}
	return result
}

const alphanumerics = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a pseudo-random string of length n.
func randomEventId(src int64, n int) string {
	randSrc := rand.NewSource(src)
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumerics[randSrc.Int63()%int64(len(alphanumerics))]
	}
	return string(b)
}
