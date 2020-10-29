package helpers

import (
	crand "crypto/rand"
	"crypto/sha256"
	"fmt"
)

func ChannelURItoChannelID(feedURI string) [32]byte {
	return sha256.Sum256([]byte(feedURI))
}

type UUID string

func NewUUID() UUID {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return UUID(fmt.Sprintf("%x", b))
}
