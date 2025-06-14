package utilz

import (
	"time"

	"math/rand"
)

func checkAssertion(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}
func Assert(cond bool) {
	if !cond {
		panic("assertion failed")
	}
}
func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
func RandRange(min, max uint32) uint32 {
	return rand.Uint32()%(max-min) + min
}
