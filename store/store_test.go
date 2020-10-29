package store

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/stretchr/testify/require"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
)

func TestStore_Subscribe_FromEmpty(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	channel, ctx, cancel := setup(t)
	defer cancel()

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub, 0, Tail)

	txnID, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID)

	txnID, err = channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID)

	sub.wait()
	require.Equal(2, sub.count)
}

func TestStore_Subscribe_DetectNewWrites(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	txnID1, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID1)

	txnID2, err := channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID2)

	txnID3, err := channel.Append([]byte("message 3"))
	require.NoError(err)
	require.Equal(uint64(3), txnID3)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub, txnID2, Tail)
	sub.wait()

	require.Equal(2, sub.count)

	sub.expect(2)

	_, err = channel.Append([]byte("message 4"))
	require.NoError(err)

	_, err = channel.Append([]byte("message 5"))
	require.NoError(err)

	sub.wait()

	require.Equal(4, sub.count)

	msg, err := channel.get(uint64(3))
	require.NoError(err)
	require.Equal("message 3", string(msg))
}

func TestStore_Subscribe_Reset(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	txnID1, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID1)

	txnID2, err := channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID2)

	txnID3, err := channel.Append([]byte("message 3"))
	require.NoError(err)
	require.Equal(uint64(3), txnID3)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub, txnID2, Tail)
	sub.wait()

	require.Equal(2, sub.count)

	sub.expect(3)
	channel.Subscribe(ctx, sub, txnID1, Tail)
	sub.wait()

	require.Equal(5, sub.count)

	sub.expect(2)
	channel.Subscribe(ctx, sub, txnID1, 2)
	sub.wait()
	require.Equal(7, sub.count)

	msg, err := channel.get(uint64(3))
	require.NoError(err)
	require.Equal("message 3", string(msg))
}

func TestStore_Subscribe_ResetConcurrentAppends(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	txnID1, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID1)

	txnID2, err := channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID2)

	txnID3, err := channel.Append([]byte("message 3"))
	require.NoError(err)
	require.Equal(uint64(3), txnID3)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub, txnID2, Tail)
	sub.wait()

	require.Equal(2, sub.count)

	sub.expect(5)

	// we're now resetting the subscription while a concurrent write
	// is coming.  we can't make any guarantees about the order of
	// events here and we'll get the last message repeated.
	channel.Subscribe(ctx, sub, txnID1, Tail)
	_, err = channel.Append([]byte("message 4"))
	require.NoError(err)
	sub.wait()

	require.Equal(7, sub.count)

	msg, err := channel.get(uint64(4))
	require.NoError(err)
	require.Equal("message 4", string(msg))

	_, err = channel.get(uint64(5))
	require.Error(err)
	require.EqualError(err, badger.ErrKeyNotFound.Error())

}

func TestStore_Restore_Channel(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultConfig()
	cfg.DB = cfg.DB.
		WithDir("").                     // need to unset for in-memory
		WithValueDir("").                // need to unset for in-memory
		WithInMemory(true).              // no cleanup
		WithLoggingLevel(badger.WARNING) // avoid test noise

	store, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	channelID := helpers.ChannelURItoChannelID(t.Name())
	channel, err := store.Channel(channelID)
	if err != nil {
		t.Fatalf(err.Error())
	}

	txnID, err := channel.Append([]byte("message 1"))
	require.NoError(err)
	require.Equal(uint64(1), txnID)

	txnID, err = channel.Append([]byte("message 2"))
	require.NoError(err)
	require.Equal(uint64(2), txnID)

	channel, err = store.Channel(channelID)
	require.Equal(uint64(2), channel.txnID)

	// TODO: we don't have a Channel shutdown mechanism separate from
	// the Store
	delete(store.channels, channelID)
	channel, err = store.Channel(channelID)
	require.Equal(uint64(2), channel.txnID)
}

func setup(t *testing.T) (*Channel, context.Context, func()) {

	ctx, cancel := context.WithCancel(context.Background())

	cfg := DefaultConfig()
	cfg.DB = cfg.DB.
		WithDir("").                     // need to unset for in-memory
		WithValueDir("").                // need to unset for in-memory
		WithInMemory(true).              // no cleanup
		WithLoggingLevel(badger.WARNING) // avoid test noise

	store, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	channelID := helpers.ChannelURItoChannelID(t.Name())
	channel, err := store.Channel(channelID)
	if err != nil {
		t.Fatalf(err.Error())
	}

	return channel, ctx, cancel
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
		case <-sub.rx:
			sub.count++
			sub.wg.Done()
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
