package httpapi

import (
	"crypto/rand"
	"encoding/hex"
)

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand is unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}
