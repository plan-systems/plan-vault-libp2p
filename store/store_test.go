package store

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
)

func TestStoreSubscribe(t *testing.T) {

	require := require.New(t)

	dir, err := ioutil.TempDir("", t.Name())
	if err != nil {
		t.Fatalf(err.Error())
	}
	defer os.RemoveAll(dir) // clean up

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // shutdown the store

	cfg := DefaultConfig()
	cfg.DataDir = dir
	store, _ := New(ctx, cfg)

	channelID := helpers.ChannelURItoChannelID(t.Name())
	fmt.Printf("channelID %x\n", channelID)
	channel := store.Channel(channelID)

	txnID1, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID1)

	txnID2, err := channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID2)

	txnID3, err := channel.Append([]byte("message 3"))
	require.NoError(err)
	require.Equal(uint64(3), txnID3)

	msg, err := channel.get(txnID1)
	fmt.Println(msg)
	fmt.Println(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub, txnID2, Tail)
	sub.wait()

	require.Equal(2, sub.count)
	require.NoError(err)

}

type MockSubscriber struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	count    int
	rx       chan []byte
	wg       sync.WaitGroup
}

func NewMockSubscriber(pctx context.Context) *MockSubscriber {
	ctx, cancel := context.WithCancel(pctx)
	sub := &MockSubscriber{
		ctx:      ctx,
		cancelFn: cancel,
		rx:       make(chan []byte),
	}
	go sub.run()
	return sub
}

func (sub *MockSubscriber) Send(msg []byte) {
	sub.rx <- msg
}

func (sub *MockSubscriber) Done() {
	sub.cancelFn()
}

func (sub *MockSubscriber) ID() helpers.UUID {
	return helpers.UUID(fmt.Sprintf("%x", &sub))
}

func (sub *MockSubscriber) run() {
	for {
		select {
		case <-sub.ctx.Done():
			return
		case recv := <-sub.rx:
			sub.count++
			sub.wg.Done()
			fmt.Printf("received: %v\n", recv)
		}
	}
}

// test helper to set an expected number of entries to be sent
func (sub *MockSubscriber) expect(expected int) {
	sub.wg.Add(expected)
}

// test helper to let us block until expected number of entries
func (sub *MockSubscriber) wait() {
	sub.wg.Wait()
}
