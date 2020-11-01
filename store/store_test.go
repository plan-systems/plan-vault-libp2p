package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/stretchr/testify/require"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

func TestStore_StreamOptFlags(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	f := OptKeysOnly
	require.True(f.Has(OptKeysOnly))
	require.False(f.Has(OptSkipFirst))

	f = OptSkipFirst | OptNone
	require.True(f.Has(OptSkipFirst))
	require.Equal(OptSkipFirst, f)

	f = OptSkipFirst | OptKeysOnly
	require.True(f.Has(OptSkipFirst))
	require.True(f.Has(OptKeysOnly))

	f = OptSkipFirst | OptKeysOnly | OptFromHead
	require.True(f.Has(OptSkipFirst))
	require.True(f.Has(OptKeysOnly))
	require.True(f.Has(OptFromHead))

	f = OptSkipFirst | OptKeysOnly | OptFromHead | OptFromGenesis
	require.True(f.Has(OptSkipFirst))
	require.True(f.Has(OptKeysOnly))
	require.True(f.Has(OptFromHead))
	require.True(f.Has(OptFromGenesis))
}

func TestStore_Subscribe_FromEmpty(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	channel, ctx, cancel := setup(t)
	defer cancel()

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.firstKey(), Max: Tail, Flags: OptNone})

	_, err := channel.Append(helpers.NewEntry("message 1"))
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	idx2, err := channel.Append(ent2)
	require.NoError(err)

	sub.wait()
	require.Equal(2, sub.count())

	// the last item the subscriber got...
	require.Equal(id(ent2), id(sub.received[1]))
	require.Equal("message 2", string(sub.received[1].GetBody()))

	// which matches what we wrote for the index...
	rxMsg, err := channel.Get(channel.entryKeyFromIndex(idx2))
	require.NoError(err)
	require.Equal("message 2", string(rxMsg.GetBody()))

	// which matches what we wrote for the key
	rxMsg, err = channel.Get(channel.EntryKey(id(ent2)))
	require.NoError(err)
	require.Equal("message 2", string(rxMsg.GetBody()))
}

func TestStore_Subscribe_AfterHead(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	channel, ctx, cancel := setup(t)
	defer cancel()

	ent1 := helpers.NewEntry("message 1")
	_, err := channel.Append(ent1)
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	_, err = channel.Append(ent2)
	require.NoError(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(1)
	require.NoError(err)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: []byte{}, Max: Tail, Flags: OptFromHead | OptSkipFirst})

	// if we skip first, the new append will race with setting up the
	// subscription.  wait until we know we've done that
	time.Sleep(1 * time.Second)

	ent3 := helpers.NewEntry("message 3")
	_, err = channel.Append(ent3)
	require.NoError(err)

	sub.wait()
	require.Equal(1, sub.count())

	// the only item the subscriber got...
	require.Equal(id(ent3), id(sub.received[0]))
	require.Equal("message 3", string(sub.received[0].GetBody()))
}

func TestStore_Subscribe_DetectNewWrites(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	_, err := channel.Append(helpers.NewEntry("message 1"))
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	_, err = channel.Append(ent2)
	require.NoError(err)

	ent3 := helpers.NewEntry("message 3")
	idx3, err := channel.Append(ent3)
	require.NoError(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.EntryKey(id(ent2)), Max: Tail, Flags: OptNone})

	// the subscriber should get exactly 2...
	sub.wait()
	require.Equal(2, sub.count())

	// send 2 more...
	sub.expect(2)

	ent4 := helpers.NewEntry("message 4")
	_, err = channel.Append(ent4)
	require.NoError(err)

	ent5 := helpers.NewEntry("message 5")
	_, err = channel.Append(ent5)
	require.NoError(err)

	sub.wait()
	require.Equal(4, sub.count())

	msg, err := channel.Get(channel.entryKeyFromIndex(idx3))
	require.NoError(err)
	require.Equal("message 3", string(msg.GetBody()))

	// we should get the writes in the order they were written
	require.Equal(id(ent2), id(sub.received[0]))
	require.Equal(id(ent3), id(sub.received[1]))
	require.Equal(id(ent4), id(sub.received[2]))
	require.Equal(id(ent5), id(sub.received[3]))

}

func TestStore_Subscribe_Reset(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	ent1 := helpers.NewEntry("message 1")
	idx1, err := channel.Append(ent1)
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	idx2, err := channel.Append(ent2)
	require.NoError(err)

	ent3 := helpers.NewEntry("message 3")
	idx3, err := channel.Append(ent3)
	require.NoError(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.entryKeyFromIndex(idx2), Max: Tail, Flags: OptNone})
	sub.wait()

	require.Equal(2, sub.count())

	sub.expect(3)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.entryKeyFromIndex(idx1), Max: Tail, Flags: OptNone})

	sub.wait()

	require.Equal(5, sub.count())

	sub.expect(2)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.entryKeyFromIndex(idx1), Max: 2, Flags: OptNone})

	sub.wait()
	require.Equal(7, sub.count())

	msg, err := channel.Get(channel.entryKeyFromIndex(idx3))
	require.NoError(err)
	require.Equal("message 3", string(msg.GetBody()))

	// get the messages in the expected order, given resets
	require.Equal(id(ent2), id(sub.received[0]))
	require.Equal(id(ent3), id(sub.received[1]))
	require.Equal(id(ent1), id(sub.received[2]))
	require.Equal(id(ent2), id(sub.received[3]))
	require.Equal(id(ent3), id(sub.received[4]))
	require.Equal(id(ent1), id(sub.received[5]))
	require.Equal(id(ent2), id(sub.received[6]))

}

func TestStore_Subscribe_ResetConcurrentAppends(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	channel, ctx, cancel := setup(t)
	defer cancel()

	ent1 := helpers.NewEntry("message 1")
	idx1, err := channel.Append(ent1)
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	idx2, err := channel.Append(ent2)
	require.NoError(err)

	ent3 := helpers.NewEntry("message 3")
	_, err = channel.Append(ent3)
	require.NoError(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(2)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.entryKeyFromIndex(idx2), Max: Tail, Flags: OptNone})

	sub.wait()

	require.Equal(2, sub.count())

	sub.expect(5)

	// we're now resetting the subscription while a concurrent write
	// is coming. we can't make any guarantees about the order of
	// events here and we'll get the last message repeated.
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: channel.entryKeyFromIndex(idx1), Max: Tail, Flags: OptNone})

	ent4 := helpers.NewEntry("message 4")
	idx4, err := channel.Append(ent4)
	require.NoError(err)
	sub.wait()

	require.Equal(7, sub.count())

	msg, err := channel.Get(channel.entryKeyFromIndex(idx4))
	require.NoError(err)
	require.Equal("message 4", string(msg.GetBody()))

	idx4[len(idx4)-1] = 0xFF

	_, err = channel.Get(channel.entryKeyFromIndex(idx4))
	require.Error(err)
	require.EqualError(err, badger.ErrKeyNotFound.Error())

	// get the messages in the expected order, prior to reset
	require.Equal(id(ent2), id(sub.received[0]))
	require.Equal(id(ent3), id(sub.received[1]))

	// we can't guarantee the order with concurrent resets and
	// appends, just that the last one we see is the last one
	require.Equal(id(ent4), id(sub.received[6]))
}

func TestStore_Restore_Channel(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	store, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	channelID := helpers.ChannelURItoChannelID(t.Name())
	channel, err := store.Channel(channelID)
	if err != nil {
		t.Fatalf(err.Error())
	}

	_, err = channel.Append(helpers.NewEntry("message 1"))
	require.NoError(err)

	idx2, err := channel.Append(helpers.NewEntry("message 2"))
	require.NoError(err)

	channel, err = store.Channel(channelID)
	require.Equal(1, len(store.channels))

	// TODO: we don't have a Channel shutdown mechanism separate from
	// the Store
	delete(store.channels, channelID)
	channel, err = store.Channel(channelID)
	require.Equal(1, len(store.channels))

	msg, err := channel.Get(channel.entryKeyFromIndex(idx2))
	require.NoError(err)
	require.Equal("message 2", string(msg.GetBody()))
}

func TestStore_Subscribe_FromIndex(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	store, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	channelID := helpers.ChannelURItoChannelID(t.Name())
	channel, err := store.Channel(channelID)
	if err != nil {
		t.Fatalf(err.Error())
	}

	require.NotNil(channel)

	_, err = channel.Append(helpers.NewEntry("message 1"))
	require.NoError(err)

	ent2 := helpers.NewEntry("message 2")
	idx2, err := channel.Append(ent2)
	require.NoError(err)

	// note: intentionally out-of-order with "message 3"!
	ent4 := helpers.NewEntry("message 4")
	_, err = channel.Append(ent4)
	require.NoError(err)

	ent3 := helpers.NewEntry("message 3")
	_, err = channel.Append(ent3)
	require.NoError(err)

	sub := NewMockSubscriber(ctx)
	sub.expect(3)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: idx2, Max: 3, Flags: OptKeysOnly | OptFromIndex})

	sub.wait()
	require.Equal(3, sub.count())

	require.Equal(id(ent2), id(sub.received[0]))
	require.Equal(id(ent4), id(sub.received[1]))
	require.Equal(id(ent3), id(sub.received[2]))

	require.Equal([]byte(nil), sub.received[0].GetBody())
	require.Equal([]byte(nil), sub.received[1].GetBody())
	require.Equal([]byte(nil), sub.received[2].GetBody())

	// do it again, this time without key-only set...
	sub.expect(3)
	channel.Subscribe(ctx, sub,
		&StreamOpts{Seek: idx2, Max: 3, Flags: OptFromIndex})
	sub.wait()
	require.Equal(6, sub.count())

	require.Equal(id(ent2), id(sub.received[3]))
	require.Equal(id(ent4), id(sub.received[4]))
	require.Equal(id(ent3), id(sub.received[5]))

	require.Equal("message 2", string(sub.received[3].GetBody()))
	require.Equal("message 4", string(sub.received[4].GetBody()))
	require.Equal("message 3", string(sub.received[5].GetBody()))
}

func setup(t *testing.T) (*Channel, context.Context, func()) {

	ctx, cancel := context.WithCancel(context.Background())
	cfg := testConfig()
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

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.DB = cfg.DB.
		WithDir("").                     // need to unset for in-memory
		WithValueDir("").                // need to unset for in-memory
		WithInMemory(true).              // no cleanup
		WithLoggingLevel(badger.WARNING) // avoid test noise
	return cfg
}

func id(msg *pb.Msg) []byte {
	return msg.EntryHeader.GetEntryID()
}

func keyTxn(k []byte) uint64 {
	return binary.BigEndian.Uint64(k[64:])
}

type MockSubscriber struct {
	ctx      context.Context
	cancelFn context.CancelFunc

	lock     sync.Mutex
	received []*pb.Msg

	rx chan *pb.Msg
	wg sync.WaitGroup
}

func NewMockSubscriber(pctx context.Context) *MockSubscriber {
	ctx, cancel := context.WithCancel(pctx)
	sub := &MockSubscriber{
		ctx:      ctx,
		cancelFn: cancel,
		received: []*pb.Msg{},
		rx:       make(chan *pb.Msg),
	}
	go sub.run()
	return sub
}

func (sub *MockSubscriber) Send(msg *pb.Msg) {
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
		case rx := <-sub.rx:
			printMsg(rx)
			sub.lock.Lock()
			sub.received = append(sub.received, rx)
			sub.lock.Unlock()
			sub.wg.Done()
		}
	}
}

func (sub *MockSubscriber) count() int {
	sub.lock.Lock()
	defer sub.lock.Unlock()
	return len(sub.received)
}

func printMsg(msg *pb.Msg) {
	// fmt.Printf("%x\n", msg.EntryHeader.GetEntryID())
}

// test helper to set an expected number of entries to be sent
func (sub *MockSubscriber) expect(expected int) {
	sub.wg.Add(expected)
}

// test helper to let us block until expected number of entries
func (sub *MockSubscriber) wait() {
	sub.wg.Wait()
}
