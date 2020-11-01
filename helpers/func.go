package helpers

import (
	crand "crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	pb "github.com/plan-systems/plan-vault-libp2p/protos"
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

// NewEntry returns a new entry Msg; used for testing
// TODO: replace with a workspace-wide library function
func NewEntry(msg string) *pb.Msg {
	return &pb.Msg{
		EntryHeader: &pb.EntryHeader{EntryID: NewEntryID()},
		Body:        []byte(msg),
	}
}

// NewEntryID returns a new EntryID; used for testing only because it
// doesn't include a hash, just a random suffix
// TODO: replace with a workspace-wide library function
func NewEntryID() []byte {
	id := make([]byte, 30)
	t := timestamp()
	id[0] = byte(t >> 56)
	id[1] = byte(t >> 48)
	id[2] = byte(t >> 40)
	id[3] = byte(t >> 32)
	id[4] = byte(t >> 24)
	id[5] = byte(t >> 16)
	id[6] = byte(t >> 8)
	id[7] = byte(t)

	crand.Read(id[8:])
	return id
}

func timestamp() int64 {
	t := time.Now()
	timeFS := t.Unix() << 16
	frac := uint16((2199 * (uint32(t.Nanosecond()) >> 10)) >> 15)
	return int64(timeFS | int64(frac))
}
