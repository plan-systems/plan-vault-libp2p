/*
The Store serves as an intermediary between the Server and the p2p
Node Host. Vault clients should be able to durably write the Store without
connections to Vault peers. Both services acts as clients of the
Store and watch it for changes to stream to either the Vault client or
the host's peers.

Design notes on schema

Badger is a pure K/V store without a table/collection abstraction,
but keeping open a large number of DBs is likely to run into ulimits
usability issues (not to mention spawning large numbers of
goroutines), so we want to use a single table space.

EntryIDs are 30 bytes, sortable by timestamp and including a random
component sufficient to prevent collisions.

Because peers can come and go, we may get "historical" entries from
those peers from a large time window in the past (configurable by
the community but expected to be on the order of days and weeks, not
seconds).

Therefore, in addition to the Entries keyspace, we'll have an
ArrivalIndex keyspace (with zero-values) which gives us ordered keys
for when entries were received by this peer. A client that comes
online can use its last ArrivalIndex to ask the vault for the keys
that it missed while disconnected.

The ArrivalIndex can be "aged out".

Entry key schema (63 bytes):

  bytes  00:31  channel ID (sha256)
  byte   32     table space (Entries) + reserved control bits
  bytes  34:63  entry ID

ArrivalIndex key schema (69 bytes):

  bytes  00:31  channel ID (sha256)
  byte   32     table space (ArrivalIndex) + reserved control bits
  bytes  33:38  timestamp in unix seconds (UTC, big endian)
  bytes  39:69  entry ID

Entry value schema is a protos.Msg w/ Op and ReqID left unset.

*/
package store

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/apex/log"
	metrics "github.com/armon/go-metrics"
	badger "github.com/dgraph-io/badger/v2"
	"google.golang.org/protobuf/proto"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

type Store struct {
	db  *badger.DB
	ctx context.Context
	log *log.Entry

	updateCh chan *Channel
	channels map[ChannelID]*Channel
	lock     sync.RWMutex
}

const (
	Tail            = math.MaxUint64
	keySpaceEntries = 0x01
	keySpaceArrival = 0x02
)

var (
	ErrorInvalidEntryID = errors.New("invalid entry ID")
	ErrorKeyExists      = errors.New("entry ID already exists")
	ErrorKeyNotFound    = errors.New("key not found")
)

type ChannelID = [32]byte
type StoreKey = []byte
type ArrivalKey = []byte

func New(ctx context.Context, cfg *Config) (*Store, error) {

	db, err := badger.Open(cfg.DB)
	if err != nil {
		return nil, err
	}
	store := &Store{
		db:       db,
		ctx:      ctx,
		log:      cfg.Log,
		updateCh: nil,
		channels: map[ChannelID]*Channel{},
	}
	if cfg.HasDiscovery {
		store.updateCh = make(chan *Channel)
	}

	go func() { <-ctx.Done(); store.close() }()
	return store, err
}

func (s *Store) close() {
	s.db.Close()
}

func (s *Store) ChannelUpdates() chan *Channel {
	return s.updateCh
}

// Channel creates a new channel and starts its watcher, or returns
// one we've registered previously.
func (s *Store) Channel(id ChannelID) (*Channel, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if channel, ok := s.channels[id]; ok {
		return channel, nil
	}

	prefix := make([]byte, 33)
	copy(prefix, id[:])
	prefix[32] = keySpaceEntries

	indexPrefix := make([]byte, 33)
	copy(indexPrefix, id[:])
	indexPrefix[32] = keySpaceArrival

	channel := &Channel{
		id:          id,
		prefix:      prefix,
		indexPrefix: indexPrefix,
		store:       s,
	}

	channel.subscribers = map[helpers.UUID]*subscriber{}
	s.channels[id] = channel
	if s.updateCh != nil {
		go func() { s.updateCh <- channel }()
	}
	channel.watch()
	return channel, nil
}

// Get accesses a key directly, implemented for p2p snapshot and
// restore, as well as debugging and testing.
func (s *Store) Get(key StoreKey) (*pb.Msg, error) {
	msg := &pb.Msg{}
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return ErrorKeyNotFound
			}
			return err
		}
		err = item.Value(func(val []byte) error {
			return proto.Unmarshal(val, msg)
		})
		return err
	})
	return msg, err
}

// Upsert writes an entry directly, implemented for p2p snapshot and
// restore, as well as debugging and testing. It does not write into
// the index. The caller is responsible for anything it cares about in
// the EntryHeader.
func (s *Store) Upsert(key StoreKey, entry *pb.Msg) error {
	m, err := proto.Marshal(entry)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		ent := badger.NewEntry(key, m)
		return txn.SetEntry(ent)
	})
}

// Channel is an abstraction around the Store that tracks state for subscribers
type Channel struct {
	id  ChannelID
	uri string

	prefix      []byte
	indexPrefix []byte

	store *Store

	subscribers map[helpers.UUID]*subscriber
	sLock       sync.RWMutex
}

func (c *Channel) URI() string {
	return c.uri
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

// lastKey is used to get the last entry ID after a reconnection,
// assuming the client doesn't ask for a replay.
func (c *Channel) lastKey() (StoreKey, error) {
	var key []byte
	err := c.store.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		// this check looks redundant and fussy but is definitely
		// required for correct reverse iteration in badger, otherwise
		// you get no results (see their FAQ)
		for it.Rewind(); it.Valid(); it.Next() {
			if !it.ValidForPrefix(c.prefix) {
				continue
			}
			key = it.Item().Key()
			return nil
		}
		return nil // new channel
	})
	return key, err

}

// Append creates a new entry and returns the ArrivalIndex key for
// that entry, so that clients can use it as a bookmark
func (c *Channel) Append(entry *pb.Msg) (StoreKey, error) {
	id, err := c.entryID(entry)
	if err != nil {
		return nil, err
	}

	key := c.EntryKey(id)
	m, err := proto.Marshal(entry)
	if err != nil {
		return key, err
	}
	arrivalKey := c.arrivalKey(id)

	err = c.store.db.Update(func(txn *badger.Txn) error {
		// badger can't give us "append-only", so we need to enforce
		// that we don't have another entry for this key.
		//
		// Note: we can't guarantee ordering in case of a tight race
		// because the check happens within a snapshot. If the client
		// verifies the content hash portion of the key before
		// accepting entries, at worst an attacker can DoS writes,
		// assuming they can observe entry keys in transit (but we
		// should use TLS)
		_, err := txn.Get(key)
		if err == nil {
			return ErrorKeyExists
		}
		if err != badger.ErrKeyNotFound {
			return err
		}

		ent := badger.NewEntry(key, m)
		err = txn.SetEntry(ent)
		if err != nil {
			return err
		}

		// TODO: use WithTTL to expire index keys?
		index := badger.NewEntry(arrivalKey, nil)
		return txn.SetEntry(index)
	})
	return arrivalKey, err
}

func (c *Channel) entryID(entry *pb.Msg) ([]byte, error) {
	id := entry.GetEntryHeader().GetEntryID()
	if id == nil {
		return []byte{}, ErrorInvalidEntryID
	}
	return id, nil
}

func (c *Channel) EntryKey(entryID []byte) []byte {
	k := make([]byte, 63)
	copy(k, c.prefix)
	copy(k[33:], entryID)
	return k
}

func (c *Channel) arrivalKey(entryID []byte) []byte {
	k := make([]byte, 69)
	copy(k, c.indexPrefix)

	ts := time.Now().Unix() << 16
	k[33] = byte(ts >> 56)
	k[34] = byte(ts >> 48)
	k[35] = byte(ts >> 40)
	k[36] = byte(ts >> 32)
	k[37] = byte(ts >> 24)
	k[38] = byte(ts >> 16)

	copy(k[39:], entryID)
	return k
}

func (c *Channel) entryKeyFromIndex(index []byte) []byte {
	k := make([]byte, 63)
	copy(k, c.prefix)
	copy(k[33:], index[39:])
	return k
}

func (c *Channel) entryIDFromIndex(index []byte) []byte {
	k := make([]byte, 30)
	copy(k, index[39:])
	return k
}

func (c *Channel) entryIDFromKey(key []byte) []byte {
	return key[33:]
}

// firstKey returns the genesis key for the channel
func (c *Channel) firstKey() StoreKey {
	k := make([]byte, 30)
	copy(k, c.prefix) // TODO: do we need one for the ArrivalIndex?
	return k
}

// Get accesses a key directly, implemented for p2p snapshot and
// restore, as well as debugging and testing.
func (c *Channel) Get(key StoreKey) (*pb.Msg, error) {
	return c.store.Get(key)
}

// Subscribe sets up a subscription that will fire the target's Publish
// callback with each entry between the `start` and `max` IDs.
func (c *Channel) Subscribe(pctx context.Context, target SubscriptionTarget, opts *StreamOpts) {
	c.sLock.Lock()
	defer c.sLock.Unlock()

	// TODO: should we allow two subscribers per stream per channel?
	// one for entries and one for index?
	id := target.ID()
	if oldSub, ok := c.subscribers[id]; ok {
		oldSub.reset(opts)
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
		resetRx: make(chan *StreamOpts),
	}

	c.subscribers[sub.id] = sub
	metrics.IncrCounter([]string{"channel", "subscribers"}, 1)

	if opts.fromIndex() {
		go sub.readFromIndex(opts)
	} else {
		go sub.read(opts)
	}
}

type SubscriptionTarget interface {
	Publish(*pb.Msg)
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
	resetRx chan *StreamOpts
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

type StreamOpts struct {
	Seek  []byte
	Max   uint64
	Flags StreamOptFlags
}

type StreamOptFlags uint8

const (
	OptNone     StreamOptFlags = 0
	OptKeysOnly StreamOptFlags = 1 << iota
	OptSkipFirst
	OptFromHead
	OptFromGenesis
	OptFromIndex
)

func (opts StreamOpts) keysOnly() bool    { return opts.Flags&OptKeysOnly != 0 }
func (opts StreamOpts) skipFirst() bool   { return opts.Flags&OptSkipFirst != 0 }
func (opts StreamOpts) fromHead() bool    { return opts.Flags&OptFromHead != 0 }
func (opts StreamOpts) fromGenesis() bool { return opts.Flags&OptFromGenesis != 0 }
func (opts StreamOpts) fromIndex() bool   { return opts.Flags&OptFromIndex != 0 }

func (s *subscriber) reset(r *StreamOpts) {
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
func (sub *subscriber) read(opts *StreamOpts) error {
	if opts == nil {
		return nil
	}
	defer sub.unsubscribe()
	var count uint64
	var dirty bool
	var lastSeen []byte

	if opts.fromHead() {
		key, err := sub.channel.lastKey()
		if err != nil {
			return err
		}
		opts.Seek = key
	}

	key := opts.Seek
	max := opts.Max
	if opts.skipFirst() {
		lastSeen = key
	}

	iterate := func() error {
		return sub.channel.store.db.View(func(txn *badger.Txn) error {
			dirty = false // by this point we have a snapshot, so reset dirty flag
			iOpts := badger.DefaultIteratorOptions
			iOpts.PrefetchSize = 10
			iOpts.Prefix = sub.channel.prefix
			it := txn.NewIterator(iOpts)
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

					if reset.fromHead() {
						k, err := sub.channel.lastKey()
						if err != nil {
							return err
						}
						reset.Seek = k
					}
					key = reset.Seek
					max = reset.Max
					if opts.skipFirst() {
						lastSeen = key
					}

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
					msg := &pb.Msg{}
					if opts.keysOnly() {
						key = item.Key()
						msg.EntryHeader = &pb.EntryHeader{
							EntryID: sub.channel.entryIDFromKey(key)}
						sub.target.Publish(msg)
						lastSeen = key
						count++
					} else {
						err := item.Value(func(v []byte) error {
							err := proto.Unmarshal(v, msg)
							if err != nil {
								return err
							}
							sub.target.Publish(msg)
							key = item.Key()
							lastSeen = key
							count++
							metrics.IncrCounterWithLabels(
								[]string{"channel", "publish"}, 1,
								[]metrics.Label{{Name: "channelID", Value: sub.channel.uri}})
							return nil
						})
						if err != nil {
							return err
						}
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

// readFromIndex iterates over the ArrivalIndex so that clients can
// catch up after having been offline.
func (sub *subscriber) readFromIndex(opts *StreamOpts) error {
	defer sub.unsubscribe()
	var count uint64
	key := opts.Seek
	max := opts.Max // typically will be Tail

	return sub.channel.store.db.View(func(txn *badger.Txn) error {
		iOpts := badger.DefaultIteratorOptions
		iOpts.PrefetchSize = 10
		iOpts.Prefix = sub.channel.indexPrefix
		it := txn.NewIterator(iOpts)
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
				// the readFromIndex just throws this away, but it
				// does need to consume it to avoid locking
				// TODO: fix this, it's super gross!
			default:
				if !it.Valid() {
					// end of iterator, but this only means we've run out of
					// items in this snapshot, not that no more have landed
					// while we were iterating
					return nil
				}
				item := it.Item()
				msg := &pb.Msg{}
				if opts.keysOnly() {
					key = sub.channel.entryIDFromIndex(item.Key())
					msg.EntryHeader = &pb.EntryHeader{EntryID: key}
					sub.target.Publish(msg)
				} else {
					key = sub.channel.entryKeyFromIndex(item.Key())
					item, err := txn.Get(key)
					if err != nil {
						return err
					}
					err = item.Value(func(v []byte) error {
						err := proto.Unmarshal(v, msg)
						if err != nil {
							return err
						}
						sub.target.Publish(msg)
						return nil
					})
					if err != nil {
						return err
					}
				}
				count++
				it.Next()
			}
		}
	})
}
