package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"sync"

	badger "github.com/dgraph-io/badger/v2"
	"github.com/plan-systems/plan-vault-libp2p/helpers"
)

/*

The Store serves as an intermediary between the Server and the p2p
Node Host. Vault clients should be able to durably write the Store without
connections to Vault peers. Both services acts as clients of the
Store and watch it for changes to stream to either the Vault client or
the host's peers.

Design notes on schema:

* badger is a pure K/V store without a table/collection abstraction,
  but keeping open a large number of DBs is likely to run into ulimits
  usability issues (not to mention spawning large numbers of
  goroutines), so we want to use a single table space.

* EntryIDs are 30 bytes, sortable by timestamp and including a random
  component sufficient to prevent collisions.

* Because peers can come and go, we may get "historical" entries from
  those peers from a large time window in the past (configurable by
  the community but expected to be on the order of days and weeks, not
  seconds).

* Therefore, in addition to the Entries keyspace, we'll have an
  ArrivalIndex keyspace (with zero-values) which gives us ordered keys
  for when entries were received *by this peer*. A client that comes
  online can use its last ArrivalIndex to ask the vault for the keys
  that it missed while disconnected.

* The ArrivalIndex can be "aged out"

* Entry key schema (63 bytes):
  * bytes 00:31 channel ID (sha256)
  * byte  32    table space (Entries) + reserved control bits
  * bytes 34:63 entry ID

* ArrivalIndex key schema (68 bytes):
  * bytes 00:31 channel ID (sha256)
  * byte  32    table space (ArrivalIndex) + reserved control bits
  * bytes 33:38 timestamp in unix seconds (UTC, big endian)
  * bytes 39:68 entry ID

* Entry value schema is a protos.Msg w/ Op and ReqID left unset

*/

type Store struct {
	id  PeerID
	db  *badger.DB
	ctx context.Context

	channels map[ChannelID]*Channel
	lock     sync.Mutex
}

const Tail = math.MaxUint64

type ChannelID = [32]byte
type PeerID = [32]byte
type StoreKey = []byte

func New(ctx context.Context, cfg Config) (*Store, error) {

	db, err := badger.Open(cfg.DB)
	if err != nil {
		return nil, err
	}
	store := &Store{
		id:       cfg.PeerID,
		db:       db,
		ctx:      ctx,
		channels: map[ChannelID]*Channel{},
	}

	go func() { <-ctx.Done(); store.close() }()
	return store, err
}

func (s *Store) close() {
	s.db.Close()
}

// Channel creates a new channel and starts its watcher, or returns
// one we've registered previously.
func (s *Store) Channel(id ChannelID) (*Channel, error) {

	s.lock.Lock()
	defer s.lock.Unlock()

	if channel, ok := s.channels[id]; ok {
		return channel, nil
	}

	var b bytes.Buffer
	b.Write(id[:])
	b.Write(s.id[:])

	prefix := make([]byte, 64)
	b.Read(prefix)

	channel := &Channel{
		id:     id,
		prefix: prefix,
		store:  s,
	}
	txnID, err := channel.restoreTxnID()
	if err != nil {
		return nil, err
	}
	channel.txnID = txnID

	channel.subscribers = map[helpers.UUID]*subscriber{}
	s.channels[id] = channel
	channel.watch()
	return channel, nil
}

// Channel is an abstraction around the Store that tracks state for txnIDs
type Channel struct {
	id     ChannelID
	prefix []byte // 64-byte (channel + peer ID)
	store  *Store

	txnID uint64
	lock  sync.Mutex

	subscribers map[helpers.UUID]*subscriber
	sLock       sync.RWMutex
}

// watch wraps badger's DB.Subscribe(), broadcasting a
// notification of all DB updates for a given channel prefix to all
// subscribers to let them know their iterators are now dirty.
//
// TODO: this might end up being expensive if we have a lot of live
// channels, so it might be better to have a single subscriber for the
// whole DB ("*" prefix) but then to notify the subscribers we need to
// read each KVList to check the prefix.
func (c *Channel) watch() {
	go c.store.db.Subscribe(c.store.ctx,
		func(_ *badger.KVList) error {
			// on any update, we broadcast a notification to all subscribers.
			c.sLock.RLock()
			defer c.sLock.RUnlock()
			for _, subscriber := range c.subscribers {
				subscriber.notify()
			}
			return nil
		}, c.prefix)
}

// restoreTxnID is used to get the last txnID after a Channel restart
// or after the Vault is restarted. This is not safe to use outside of
// the Store.Channel method
func (c *Channel) restoreTxnID() (uint64, error) {
	var txnID uint64
	err := c.store.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		var err error
		defer it.Close()

		// this check looks redundant and fussy but is definitely
		// required for correct reverse iteration in badger, otherwise
		// you get no results (see their FAQ)
		for it.Rewind(); it.Valid(); it.Next() {
			if !it.ValidForPrefix(c.prefix) {
				continue
			}
			item := it.Item()
			k := item.Key()
			txnID = binary.BigEndian.Uint64(k[64:])
			return err
		}
		return nil // new channel
	})
	return txnID, err

}

// Append creates a new entry and returns the storage txnID for that
// entry.
func (c *Channel) Append(entry []byte) (StoreKey, error) {
	var key StoreKey
	err := c.store.db.Update(func(txn *badger.Txn) error {
		key, _ = c.NextKey()
		e := badger.NewEntry(key, entry)
		err := txn.SetEntry(e)
		return err
	})
	return key, err
}

// NextKey increments the internal txnID and returns the key for it
func (c *Channel) NextKey() (StoreKey, uint64) {
	c.lock.Lock()
	c.txnID++
	c.lock.Unlock()
	return c.KeyFor(c.txnID), c.txnID
}

// LastKey returns the head key of the channel
func (c *Channel) LastKey() StoreKey {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.KeyFor(c.txnID)
}

// KeyAfter returns the next key after the argument for this channel
// without advancing the internal transaction ID
func (c *Channel) KeyAfter(k StoreKey) StoreKey {
	txnID := binary.BigEndian.Uint64(k[64:])
	txnID++
	return c.KeyFor(txnID)
}

// FirstKey returns the genesis key for the channel
func (c *Channel) FirstKey() StoreKey {
	return c.KeyFor(0)
}

// KeyFor generates a key for a specific txnID
func (c *Channel) KeyFor(txnID uint64) StoreKey {
	k := make([]byte, 72)
	copy(k, c.prefix)
	binary.BigEndian.PutUint64(k[64:], txnID)
	return k
}

// get accesses a key directly, implemented mostly for debugging and
// testing purposes
func (c *Channel) Get(key StoreKey) ([]byte, error) {
	var result []byte
	err := c.store.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			result = val[:]
			return nil
		})
		return nil
	})
	return result, err
}

// Subscribe sets up a subscription that will fire the target's Send
// callback with each entry between the `start` and `max` IDs.
func (c *Channel) Subscribe(pctx context.Context, target SubscriptionTarget, start *StreamStart) {
	c.sLock.Lock()
	defer c.sLock.Unlock()

	id := target.ID()

	if oldSub, ok := c.subscribers[id]; ok {
		oldSub.reset(start)
		return
	}

	ctx, cancel := context.WithCancel(pctx)
	sub := &subscriber{
		id:      id,
		ctx:     ctx,
		cancel:  cancel,
		channel: c,
		target:  target,
		rx:      make(chan struct{}),
		resetRx: make(chan *StreamStart),
	}

	c.subscribers[sub.id] = sub
	go sub.read(start)
}

type SubscriptionTarget interface {
	Send([]byte)
	Done()
	ID() helpers.UUID
}

type subscriber struct {
	id      helpers.UUID
	ctx     context.Context
	cancel  context.CancelFunc
	channel *Channel
	target  SubscriptionTarget
	rx      chan struct{}
	resetRx chan *StreamStart
}

// TODO: once a channel has no subscribers, we should close it so
// that we don't have to run its broadcaster
func (sub *subscriber) unsubscribe() {
	sub.channel.sLock.Lock()
	defer sub.channel.sLock.Unlock()
	delete(sub.channel.subscribers, sub.id)
}

func (s *subscriber) notify() {
	s.rx <- struct{}{}
}

type StreamStart struct {
	Start   []byte
	Max     uint64
	IdsOnly bool
}

func (s *subscriber) reset(r *StreamStart) {
	s.resetRx <- r
}

// read drives the subscription.
//
// Badger DB has snapshot isolation for all views, so we can't just
// continuously iterate to pick up new entries, and the badger
// Subscribe API can only get us new entries and not historical ones.
//
// Instead, we'll iterate over a snapshot and the subscription's
// channel will notify us if any new entries have been written, which
// tells us that the iterator is "dirty" and we can start a new
// iterator once we reach the end if we still want more entries.
//
// This coaleces multiple update notifications that happen within a
// single iterator, but also lets us block if we're not dirty so that
// we're not continuously iterating if there's no incoming writes.
func (sub *subscriber) read(start *StreamStart) error {
	if start == nil {
		return nil
	}
	defer sub.unsubscribe()
	var count uint64
	var dirty bool
	var lastSeen []byte
	key := start.Start
	max := start.Max

	iterate := func() error {
		return sub.channel.store.db.View(func(txn *badger.Txn) error {
			dirty = false // by this point we have a snapshot, so reset dirty flag
			opts := badger.DefaultIteratorOptions
			opts.PrefetchSize = 10
			opts.Prefix = sub.channel.prefix
			it := txn.NewIterator(opts)
			defer it.Close()

			it.Seek(key)
			for {
				if count >= max {
					return nil
				}
				select {
				case <-sub.ctx.Done():
					return nil
				case <-sub.rx:
					dirty = true
				case reset := <-sub.resetRx:
					if reset == nil {
						return nil
					}
					dirty = true
					key = reset.Start
					max = reset.Max
					return nil
				default:
					if !it.Valid() {
						// end of iterator, but this only means we've run out of
						// items in this snapshot, not that no more have landed
						// while we were iterating
						return nil
					}
					item := it.Item()
					if bytes.Equal(item.Key(), lastSeen) {
						// drop dupes so we can seek to the last key
						// seen in the next iteration without
						// advancing to a key that might not exist
						it.Next()
						continue
					}
					err := item.Value(func(v []byte) error {
						// TODO: handle keys-only case
						// TODO: we need to copy any values we send
						sub.target.Send(v)
						key = item.Key()
						lastSeen = key
						count++
						return nil
					})
					if err != nil {
						return err
					}
					it.Next()
				}
			}
		})
	}

	for {
		err := iterate()
		if err != nil {
			return err
		}
		if count >= max {
			return nil
		}
		if !dirty {
			// if we've reached here, we're not dirty but not done either,
			// so we need to wait for a signal to start a new iterator
			select {
			case <-sub.ctx.Done():
				return nil
			case <-sub.rx:
			}
		}

	}
}
