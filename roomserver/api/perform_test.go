package api

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
)

func BenchmarkPrevEventIDs(b *testing.B) {
	for _, x := range []int64{1, 10, 100, 500, 1000, 2000} {
		benchPrevEventIDs(b, int(x))
	}
}

func benchPrevEventIDs(b *testing.B, count int) {
	bwExtrems := generateBackwardsExtremities(b, count)
	backfiller := testPerformBackfillRequest{
		BackwardsExtremities: bwExtrems,
	}

	b.Run(fmt.Sprintf("Original%d", count), func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prevIDs := backfiller.originalPrevEventIDs()
			_ = prevIDs
		}
	})

	b.Run(fmt.Sprintf("FirstAttempt%d", count), func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prevIDs := backfiller.firstAttemptPrevEventIDs()
			_ = prevIDs
		}
	})

	b.Run(fmt.Sprintf("UsingMap%d", count), func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prevIDs := backfiller.prevEventIDsUsingMap()
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

// PerformBackfillRequest is a request to PerformBackfill.
type testPerformBackfillRequest struct {
	BackwardsExtremities map[string][]string `json:"backwards_extremities"`
}

func (r *testPerformBackfillRequest) originalPrevEventIDs() []string {
	var prevEventIDs []string
	for _, pes := range r.BackwardsExtremities {
		prevEventIDs = append(prevEventIDs, pes...)
	}
	prevEventIDs = util.UniqueStrings(prevEventIDs)
	return prevEventIDs
}

func (r *testPerformBackfillRequest) firstAttemptPrevEventIDs() []string {
	// Collect 1k eventIDs, if possible, they may be cleared out below
	maxPrevEventIDs := len(r.BackwardsExtremities) * 3
	if maxPrevEventIDs > 2000 {
		maxPrevEventIDs = 2000
	}
	prevEventIDs := make([]string, 0, maxPrevEventIDs)
	for _, pes := range r.BackwardsExtremities {
		prevEventIDs = append(prevEventIDs, pes...)
		if len(prevEventIDs) > 1000 {
			break
		}
	}
	prevEventIDs = util.UniqueStrings(prevEventIDs)
	// If we still have > 100 eventIDs, only return the first 100
	if len(prevEventIDs) > 100 {
		return prevEventIDs[:100]
	}
	return prevEventIDs
}

func (r *testPerformBackfillRequest) prevEventIDsUsingMap() []string {
	var uniqueIDs map[string]struct{}
	if len(r.BackwardsExtremities) > 100 {
		uniqueIDs = make(map[string]struct{}, 100)
	} else {
		uniqueIDs = make(map[string]struct{}, len(r.BackwardsExtremities))
	}

outerLoop:
	for _, pes := range r.BackwardsExtremities {
		for _, evID := range pes {
			uniqueIDs[evID] = struct{}{}
			if len(uniqueIDs) >= 100 {
				break outerLoop
			}
		}
	}

	result := make([]string, len(uniqueIDs))
	i := 0
	for evID := range uniqueIDs {
		result[i] = evID
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
	bwExtrems := generateBackwardsExtremities(t, 10)
	backfiller := PerformBackfillRequest{
		BackwardsExtremities: bwExtrems,
	}

	// prevIDs as generated for 10 backwards extremities
	wantPrevEventIDs := []string{"1ExBQDVcBj", "A9HQt03Ink", "BRUmuwogIq", "BUzb5KBQBO", "Ibyq7BhmYX", "MIMMBA6e9D", "NEQe6xuOjj", "ONRhfKsUOH", "byJR3RCsab", "eCfa30nG7H", "m3OCJNYcS4", "rMWhKXFs9K"}
	prevIDs := backfiller.PrevEventIDs()
	assert.Equal(t, wantPrevEventIDs, prevIDs)

	// prevIDs as generated for 100 backwards extremities
	backfiller.BackwardsExtremities = generateBackwardsExtremities(t, 100)
	wantPrevEventIDs = []string{"0QWd8TuUHD", "159jkpFN68", "19CrIDkTTb", "1ExBQDVcBj", "1kFAed76vN", "2xKOixxmhS", "3ApLwSwkH3", "3wszjJPW9A", "4qUOTgFKxM", "58tM6RUsRu", "5OQIDNOr4M", "5sJVbLeaKw", "6Vg95rTgUX", "6YxCUbAR9y", "6nZZd4kufM", "7REdWyhuE2", "86a1U3dnjy", "8l2IYAMcdF", "A9HQt03Ink", "AXOrOWTA0j", "AvxnvTLm8K", "AwQiSuIEbH", "BRUmuwogIq", "BUzb5KBQBO", "BrHEgialwW", "C4WNO7VSn2", "CBctYVoacn", "CN3mvQ6E6W", "E7nX8EQT1B", "ES4tNDW0wx", "FS9gV2aYvC", "FZCU0siAER", "FjXa7mqfXn", "G4q5qGdMQb", "GCxaYfmh7s", "Gs9MlFL3Fd", "H0Mzqei69C", "HITwTlAE4j", "IZ3P3uFPu6", "Ibyq7BhmYX", "JMqrpZJT9x", "JVVY34qwhB", "JZQagbldw2", "LJfQVtSU6n", "MIMMBA6e9D", "NEQe6xuOjj", "NlDSaJjceb", "ONRhfKsUOH", "OtptJ4GhQ2", "Qn1XPQhpgz", "SE3y6JniXH", "SlHpqJdjXN", "SqeoLYdQvf", "TmyExrsLR8", "Tvw8iGKHOz", "U9PldtLkuE", "UidzU2YW7G", "VkWDn232Qy", "Wbl4KHc2w0", "XL5mt8EKCt", "ZZ659s7Mwy", "ZmC0h0G6Mf", "anjChmyqJJ", "byJR3RCsab", "eCfa30nG7H", "eooGdExorQ", "esVxgLwWeD", "f0hxkMzETL", "fl5rrUR3Un", "gSoogyeREZ", "gc7zclHt95", "h9ncUeqMAD", "ha936wz0tY", "imnKEUbBlK", "lZRAlpDwpN", "lZy9etvZzj", "lwYHvKrxh0", "m3OCJNYcS4", "m8GagfYqOM", "ni3n5Y56zH", "nxz84Ndaqh", "oUJyu78LB6", "q0mUbgGerE", "qjRbAktvKw", "qmzKjSmsFs", "qncDkkqgi7", "rMWhKXFs9K", "sB8pmOdISV", "sXRWyyQlSi", "t4VYOaIWrg", "t97LRflvAD", "tZUvZ2YXy9", "ta1QyRKnUj", "vHmrrLVkpk", "vxxXjxb8lx", "wvU4kvV1WB", "x8k42j3IXJ", "xPcwjlG49n", "yHFB0bx2IA", "ytgBjHKSes"}
	prevIDs = backfiller.PrevEventIDs()
	assert.Equal(t, wantPrevEventIDs, prevIDs)
}
