package keyring

/*
The `keyring` package is a standalone implementation of the PLAN
client/pnode protocol embeded in the vault, but intended to be
pulled out as a freestanding library that can be consumed by the
actual client and pnode application
*/

import (
	crand "crypto/rand"
	"sync"

	"github.com/libp2p/go-libp2p-core/crypto"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/nacl/sign"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

type KeyRing struct {
	identity    crypto.PrivKey
	channelKeys map[string]*KeySet
	lock        sync.RWMutex
}

func NewKeyRing() *KeyRing {
	return &KeyRing{
		channelKeys: map[string]*KeySet{},
	}
}

func NewFromFile(uri, path string) (*KeyRing, error) {
	kr := NewKeyRing()
	err := restore(kr, uri, path)
	if err != nil {
		return nil, err
	}
	return kr, nil
}

// AddCommunityKey adds a community key to the channel keyring.
// TODO: this is a placeholder until we have an embedded pnode client.
func (kr *KeyRing) AddCommunityKey(channelURI string, key CommunityKey) {
	kr.lock.Lock()
	defer kr.lock.Unlock()
	ks, ok := kr.channelKeys[channelURI]
	if !ok {
		ks = NewKeySet()
		kr.channelKeys[channelURI] = ks
	}
	ks.CommunityKey = key
}

// AddKeyPair adds this client's keypair to the channel keyring.
// TODO: this is a placeholder until we have an embedded pnode client.
func (kr *KeyRing) AddKeyPair(channelURI string, id MemberID, pub MemberPublicKey, priv MemberPrivateKey) {
	kr.lock.Lock()
	defer kr.lock.Unlock()
	ks, ok := kr.channelKeys[channelURI]
	if !ok {
		ks = NewKeySet()
		kr.channelKeys[channelURI] = ks
	}
	ks.PrivateKey = priv
	ks.PublicKey = pub
	ks.MemberID = id
	ks.setMemberKey(id, pub)
}

// GetIdentityKey gets the keyring's encoded identity key and returns
// it as a libp2p private key
func (kr *KeyRing) GetIdentityKey() crypto.PrivKey {
	return kr.identity
}

// AddMemberKey adds a different member's public key to the keyring,
// for signing verification.
// TODO: this is a placeholder until we have an embedded pnode client.
func (kr *KeyRing) AddMemberKey(channelURI string, id MemberID, pub MemberPublicKey) {
	kr.lock.Lock()
	defer kr.lock.Unlock()
	ks, ok := kr.channelKeys[channelURI]
	if !ok {
		ks = NewKeySet()
		kr.channelKeys[channelURI] = ks
	}
	ks.setMemberKey(id, pub)
}

// RemoveMemberKey removes a different member's public key to the keyring,
// for signing verification.
// TODO: this is a placeholder until we have an embedded pnode client.
func (kr *KeyRing) RemoveMemberKey(channelURI string, id MemberID) error {
	kr.lock.Lock()
	defer kr.lock.Unlock()
	ks, ok := kr.channelKeys[channelURI]
	if !ok {
		return ErrorUnknownKey
	}
	ks.removeMemberKey(id)
	return nil
}

// EncodeEntry encrypts the body for a specific channel and signs the
// result, returning a new entry Msg. Note that we have to encode an
// entry for a specific channel because a given Member can have
// different keys for different channels.
func (kr *KeyRing) EncodeEntry(body []byte, channelURI string) (*pb.Msg, error) {
	ks, ok := kr.keyset(channelURI)
	if !ok {
		return nil, ErrorUnknownChannel
	}

	cipherText, err := encryptEntry(ks.CommunityKey, body)
	if err != nil {
		return nil, err
	}
	signed := signEntry(ks.PrivateKey, cipherText)

	return &pb.Msg{
		EntryHeader: &pb.EntryHeader{
			EntryID:  helpers.NewEntryID(),
			MemberID: ks.MemberID[:],
			FeedURI:  channelURI,
		},
		Body: signed,
	}, nil
}

// DecodeEntry verifies the signature of the entry, and returns the
// decrypted body
func (kr *KeyRing) DecodeEntry(entry *pb.Msg) ([]byte, error) {

	// TODO: how can we constrain this value?
	channelURI := entry.EntryHeader.GetFeedURI()
	ks, ok := kr.keyset(channelURI)
	if !ok {
		return nil, ErrorUnknownChannel
	}

	signed := entry.GetBody()
	if signed == nil {
		return nil, ErrorInvalidSignature
	}

	memberID := entry.EntryHeader.GetMemberID()
	mID := MemberID{}
	copy(mID[:], memberID)
	sender, err := ks.getMemberKey(mID)
	if err != nil {
		return nil, ErrorInvalidSignature
	}
	verified, ok := verifyEntry(signed, sender)
	if !ok {
		return nil, ErrorInvalidSignature
	}

	return decryptEntry(verified, ks.CommunityKey)
}

// keyset is an accessor with a read lock
func (kr *KeyRing) keyset(channelURI string) (*KeySet, bool) {
	kr.lock.RLock()
	defer kr.lock.RUnlock()
	ks, ok := kr.channelKeys[channelURI]
	return ks, ok
}

// KeySet is associated with a Channel; the caller is responsible for
// tracking multiple keychains and rotating them if needed
type KeySet struct {
	CommunityKey CommunityKey
	MemberID     MemberID         // this client's Id
	PrivateKey   MemberPrivateKey // this client's private key
	PublicKey    MemberPublicKey  // this client's public key

	memberKeys map[MemberID]MemberPublicKey
	lock       sync.RWMutex
}

func NewKeySet() *KeySet {
	return &KeySet{
		memberKeys: map[MemberID]MemberPublicKey{},
	}
}

// getMemberKey is intentionally left unexported because the caller
// shouldn't ever need it
func (k *KeySet) getMemberKey(id MemberID) (MemberPublicKey, error) {
	k.lock.RLock()
	defer k.lock.RUnlock()
	key, ok := k.memberKeys[id]
	if !ok {
		return nil, ErrorUnknownKey
	}
	return key, nil
}

func (k *KeySet) setMemberKey(id MemberID, key MemberPublicKey) {
	k.lock.Lock()
	defer k.lock.Unlock()
	k.memberKeys[id] = key
}

func (k *KeySet) removeMemberKey(id MemberID) error {
	k.lock.Lock()
	defer k.lock.Unlock()
	_, ok := k.memberKeys[id]
	if !ok {
		return ErrorUnknownKey
	}
	delete(k.memberKeys, id)
	return nil
}

// ---------------------------------------
// the functions below are all pure cryptographic free functions

func decryptEntry(encrypted []byte, communityKey CommunityKey) ([]byte, error) {
	var salt [saltLen]byte
	copy(salt[:], encrypted[:saltLen])
	decrypted, ok := secretbox.Open(nil, encrypted[saltLen:], &salt, communityKey)
	if !ok {
		return nil, ErrorDecryptFailedSym
	}
	return decrypted, nil
}

func encryptEntry(communityKey CommunityKey, entry []byte) ([]byte, error) {
	var salt [saltLen]byte
	_, err := crand.Read(salt[:])
	if err != nil {
		return nil, err // wtf? we're out of entropy?
	}
	return secretbox.Seal(salt[:], entry, &salt, communityKey), nil
}

func signEntry(privateKey MemberPrivateKey, message []byte) []byte {
	signed := sign.Sign(nil, message[:], privateKey)
	return signed
}

func verifyEntry(signed []byte, pubKey MemberPublicKey) ([]byte, bool) {
	return sign.Open(nil, signed[:], pubKey)
}
