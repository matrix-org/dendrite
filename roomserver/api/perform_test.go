package api

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkPrevEventIDs(b *testing.B) {
	for _, x := range []int64{1, 10, 100, 500, 1000, 2000} {
		benchPrevEventIDs(b, int(x))
	}
}

func benchPrevEventIDs(b *testing.B, count int) {
	bwExtrems := generateBackwardsExtremities(b, count)
	backfiller := PerformBackfillRequest{
		BackwardsExtremities: bwExtrems,
	}

	b.Run(fmt.Sprintf("Original%d", count), func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prevIDs := backfiller.PrevEventIDs()
			_ = prevIDs
		}
	})
}

type testLike interface {
	Helper()
}

const randomIDCharsCount = 10

func generateBackwardsExtremities(t testLike, count int) map[string][]string {
	t.Helper()
	result := make(map[string][]string, count)
	for i := 0; i < count; i++ {
		eventID := randomEventId(int64(i))
		result[eventID] = []string{
			randomEventId(int64(i + 1)),
			randomEventId(int64(i + 2)),
			randomEventId(int64(i + 3)),
		}
	}
	return result
}

const alphanumerics = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomEventId generates a pseudo-random string of length n.
func randomEventId(src int64) string {
	randSrc := rand.NewSource(src)
	b := make([]byte, randomIDCharsCount)
	for i := range b {
		b[i] = alphanumerics[randSrc.Int63()%int64(len(alphanumerics))]
	}
	return string(b)
}

func TestPrevEventIDs(t *testing.T) {
	// generate 10 backwards extremities
	bwExtrems := generateBackwardsExtremities(t, 10)
	backfiller := PerformBackfillRequest{
		BackwardsExtremities: bwExtrems,
	}

	prevIDs := backfiller.PrevEventIDs()
	// Given how "generateBackwardsExtremities" works, this
	// generates 12 unique event IDs
	assert.Equal(t, 12, len(prevIDs))

	// generate 200 backwards extremities
	backfiller.BackwardsExtremities = generateBackwardsExtremities(t, 200)
	prevIDs = backfiller.PrevEventIDs()
	// PrevEventIDs returns at max 100 event IDs
	assert.Equal(t, 100, len(prevIDs))
}
