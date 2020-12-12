package keyring

import (
	"errors"

	"github.com/libp2p/go-libp2p-core/crypto"
	libpeer "github.com/libp2p/go-libp2p-core/peer"
)

/*
This file includes error types and type aliases for arrays along with
helpers that provide a typesafe way of handling keys and converting
between PLAN protobuf types and libp2p types
*/

var (
	ErrorDecryptFailedAsym = errors.New("assymmetric decryption failed")
	ErrorDecryptFailedSym  = errors.New("symmetric decryption failed")
	ErrorInvalidSignature  = errors.New("invalid or missing signature")
	ErrorUnknownChannel    = errors.New("unknown channel")
	ErrorUnknownKey        = errors.New("unknown key")
)

const saltLen = 24

type CommunityKey *[32]byte
type MemberPublicKey *[32]byte
type MemberPrivateKey *[64]byte
type MemberID [20]byte

func MemberPublicKeyFromP2PKey(k crypto.PubKey) (MemberPublicKey, error) {
	b, err := k.Raw()
	if err != nil {
		var pub MemberPublicKey
		return pub, err
	}
	return MemberPublicKeyFromBytes(b), nil
}

func MemberPrivateKeyFromP2PKey(k crypto.PrivKey) (MemberPrivateKey, error) {
	b, err := k.Raw()
	if err != nil {
		var priv MemberPrivateKey
		return priv, err
	}
	return MemberPrivateKeyFromBytes(b), nil
}

func MemberPrivateKeyFromBytes(b []byte) MemberPrivateKey {
	privkey := [64]byte{}
	copy(privkey[:], b)
	return MemberPrivateKey(&privkey)
}

func MemberPublicKeyFromBytes(b []byte) MemberPublicKey {
	pubkey := [32]byte{}
	copy(pubkey[:], b)
	return MemberPublicKey(&pubkey)
}

func MemberIDFromString(s string) MemberID {
	id := [20]byte{}
	copy(id[:], []byte(s))
	return MemberID(id)
}

func MemberIDFromP2PPubKey(k crypto.PubKey) (MemberID, error) {
	id, err := libpeer.IDFromPublicKey(k)
	if err != nil {
		return MemberID{}, err
	}
	return MemberIDFromString(string(id)), nil
}

func DecodeP2PPrivKey(encoded string) (crypto.PrivKey, error) {
	b, err := crypto.ConfigDecodeKey(encoded)
	if err != nil {
		return nil, err
	}
	key, err := crypto.UnmarshalPrivateKey(b)
	if err != nil {
		return nil, err
	}
	return key, nil
}
