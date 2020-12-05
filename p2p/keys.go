package p2p

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/libp2p/go-libp2p-core/crypto"
)

// inMemory lets us configure keys for testing that live in memory and
// get thrown out once we're done with them
const inMemory = ":memory:"

// getKeys returns a public/private keypair from the path on disk, or
// creates one at that path if one does not exist
func getKeys(path string) (crypto.PrivKey, crypto.PubKey, error) {

	if path == inMemory {
		return generateKey()
	}

	priv, err := loadKey(path)
	if err == nil {
		pub := priv.GetPublic()
		return priv, pub, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}

	// first time we started this vault, create and save a new key
	priv, pub, err := generateKey()
	if err != nil {
		return nil, nil, err
	}
	err = saveKey(priv, *keyFile)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

func loadKey(path string) (crypto.PrivKey, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return decodeKey(string(b))
}

func saveKey(key crypto.PrivKey, path string) error {
	s, err := encodeKey(key)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0400)
	if err != nil {
		return err
	}
	defer f.Close()
	wrote, err := f.WriteString(s)
	if err != nil {
		return fmt.Errorf("could not write key: %w", err)
	}
	if wrote != len(s) {
		return fmt.Errorf("partial write of key")
	}
	return nil
}

func encodeKey(key crypto.PrivKey) (string, error) {
	b, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return "", err
	}
	return crypto.ConfigEncodeKey(b), nil
}

func decodeKey(encoded string) (crypto.PrivKey, error) {
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

func generateKey() (crypto.PrivKey, crypto.PubKey, error) {
	return crypto.GenerateKeyPair(
		crypto.Ed25519, // Select your key type. Ed25519 are nice short
		-1,             // Select key length when possible (i.e. RSA).
	)
}
