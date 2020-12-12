package keyring

import (
	crand "crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyRing_Roundtrip(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	kr := testKeyRing(t.Name())
	require.NotNil(kr)
	body := []byte("something of interest")

	msg, err := kr.EncodeEntry(body, t.Name()+"unknown")
	require.Error(err, ErrorUnknownChannel)
	require.Nil(msg)

	msg, err = kr.EncodeEntry(body, t.Name())
	require.NoError(err)

	decoded, err := kr.DecodeEntry(msg)
	require.NoError(err)

	require.Equal(decoded, body)
}

// testKeyRing returns a KeyRing populated with fully-populated
// keysets for each channel URI provided
func testKeyRing(uris ...string) *KeyRing {
	kr := NewKeyRing()
	for _, uri := range uris {
		cKey := [32]byte{}
		myPubKey := [32]byte{}
		myPrivKey := [64]byte{}
		mPubKey := [32]byte{}
		mID := [20]byte{}
		mID2 := [20]byte{}

		priv, pub, _ := generateKey()
		b, _ := pub.Raw()
		copy(myPubKey[:], b)
		b, _ = priv.Raw()
		copy(myPrivKey[:], b)

		_, mpub, _ := generateKey()
		b, _ = mpub.Raw()
		copy(mPubKey[:], b)

		crand.Read(cKey[:])
		crand.Read(mID[:])
		crand.Read(mID2[:])

		kr.identity = priv
		kr.AddCommunityKey(uri, CommunityKey(&cKey))
		kr.AddKeyPair(uri, MemberID(mID),
			MemberPublicKey(&myPubKey), MemberPrivateKey(&myPrivKey))
		kr.AddMemberKey(uri, MemberID(mID2), MemberPublicKey(&mPubKey))
	}
	return kr
}
