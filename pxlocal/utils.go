package pxlocal

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var uniqMap = make(map[string]bool)
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890") //ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func uniqName(n int) string {
	for {
		b := make([]rune, n)
		for i := range b {
			b[i] = letterRunes[rand.Intn(len(letterRunes))]
		}
		s := string(b)

		if uniqMap[s] {
			continue
		}
		uniqMap[s] = true
		return s
	}
}
