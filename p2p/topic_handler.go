package p2p

import (
	"context"
	"fmt"

	"github.com/apex/log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"google.golang.org/protobuf/proto"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

type TopicHandler struct {
	h       *Host
	id      helpers.UUID
	channel *store.Channel
	topic   *pubsub.Topic
	sub     *pubsub.Subscription
	ctx     context.Context
	cancel  context.CancelFunc
	logger  *log.Entry

	onPublish entryHandler
	onReceive entryHandler

	lastEntryID []byte
}

// entryHandler defines what the TopicHandler will do with new entries
// it gets from peers
type entryHandler func(entry *pb.Msg) error

func NewTopicHandler(h *Host, channel *store.Channel, start []byte, onRecv, onPublish entryHandler) (*TopicHandler, error) {
	uri := channel.URI()
	topic, err := h.pubsub.Join(uri)
	if err != nil {
		return nil, fmt.Errorf("could not join channel %q: %w", uri, err)
	}

	subOpts := []pubsub.SubOpt{}
	sub, err := topic.Subscribe(subOpts...)
	if err != nil {
		return nil, fmt.Errorf("could not subscribe to channel %q: %w", uri, err)
	}
	ctx, cancel := context.WithCancel(h.ctx)
	id := helpers.NewUUID()
	th := &TopicHandler{
		h:           h,
		id:          id,
		channel:     channel,
		topic:       topic,
		sub:         sub,
		ctx:         ctx,
		cancel:      cancel,
		onReceive:   onRecv,
		onPublish:   onPublish,
		lastEntryID: start,
		logger: h.log.WithFields(log.Fields{
			"id":  id,
			"uri": uri,
		}),
	}
	return th, nil
}

func (th *TopicHandler) watchTopic() {

	opts := &store.StreamOpts{Seek: th.lastEntryID, Max: store.Tail}
	th.channel.Subscribe(th.ctx, th, opts)

	for {
		msg, err := th.sub.Next(th.ctx)
		if err != nil {
			th.logger.WithError(err).Error("failed to get next message from peer")
			continue
		}
		if msg.ReceivedFrom == th.h.ID() {
			continue
		}
		entry := &pb.Msg{}
		err = proto.Unmarshal(msg.GetData(), entry)
		if err != nil {
			th.logger.
				WithFields(log.Fields{"peer": msg.ReceivedFrom}).
				WithError(err).Error("failed to decode next message from peer")
			continue
		}
		err = th.onReceive(entry)
		if err != nil {
			th.logger.WithError(err).Error("failed to handle message from peer")
		}
	}
}

// Publish is the callback required to implement Store.SubscriptionTarget.
// It gets called by the Store every time our subscription has a new entry
// to handle.
func (th *TopicHandler) Publish(entry *pb.Msg) {
	opts := []pubsub.PubOpt{} // TODO: what do we need here?

	body, err := proto.Marshal(entry)
	if err != nil {
		th.logger.WithError(err).Error("failed to encode new discovery entry")
	}
	th.topic.Publish(th.ctx, body, opts...)
	th.onPublish(entry)
	th.lastEntryID = entry.GetEntryHeader().GetEntryID()
}

// Done is required to implement Store.SubscriptionTarget
func (th *TopicHandler) Done() {
	th.sub.Cancel()
	th.cancel()
}

// ID is required to implement Store.SubscriptionTarget
func (th *TopicHandler) ID() helpers.UUID { return th.id }
