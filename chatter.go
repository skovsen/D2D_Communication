package comm

import (
	"context"
	"encoding/json"
	"log"

	"github.com/alexandrainst/agentlogic"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// BufferSize is the number of incoming messages to buffer for each topic.
const BufferSize = 128

type MessageType int

const (
	DiscoveryMessageType      = 0
	StateMessageType          = 1
	MissionMessageType        = 2
	ReorganizationMessageType = 3
	RecalculatorMessageType   = 4
	GoalMessageType           = 5
)

// Channel represents a subscription to a single PubSub topic. Messages
// can be published to the topic with Channel.Publish, and received
// messages are pushed to the Messages channel.
type Channel struct {
	// Messages is a channel of messages received from other peers in the chat room
	Messages chan *Message

	ctx      context.Context
	ps       *pubsub.PubSub
	topic    *pubsub.Topic
	sub      *pubsub.Subscription
	roomType MessageType
	path     string
	self     peer.ID
	Close    bool
}

type Message interface {
	private()
}

func (m DiscoveryMessage) private() {}
func (m StateMessage) private()     {}
func (m MissionMessage) private()   {}
func (m GoalMessage) private()      {}

// Message gets converted to/from JSON and sent in the body of pubsub messages.

type DiscoveryMessage struct {
	MessageMeta MessageMeta
	Content     agentlogic.Agent
}

type StateMessage struct {
	MessageMeta MessageMeta
	Content     agentlogic.State
}

type MissionMessage struct {
	MessageMeta MessageMeta
	Content     agentlogic.Mission
}

type GoalMessage struct {
	MessageMeta MessageMeta
	Content     agentlogic.Goal
	Position    agentlogic.Vector
	Poi         string
}

type MessageMeta struct {
	MsgType    MessageType
	SenderId   string
	SenderType agentlogic.AgentType
}

func JoinPath(ctx context.Context, ps *pubsub.PubSub, selfID peer.ID, path string, roomType MessageType) (*Channel, error) {
	topic, err := ps.Join(path)
	if err != nil {
		return nil, err
	}

	// and subscribe to it
	sub, err := topic.Subscribe()

	if err != nil {
		return nil, err
	}

	ch := &Channel{
		ctx:      ctx,
		ps:       ps,
		topic:    topic,
		sub:      sub,
		self:     selfID,
		path:     path,
		roomType: roomType,
		Close:    false,
		Messages: make(chan *Message, BufferSize),
	}
	channelsMux.Lock()
	channels[path] = ch
	channelsMux.Unlock()
	// start reading messages from the subscription in a loop
	//log.Println("Channel with path " + path + " joined, read loop started")
	go ch.readLoop()
	return ch, nil
}

func ClosePath(disconnectedAgent agentlogic.Agent, messageType MessageType) {
	var removeId string
	switch messageType {
	case MissionMessageType:
		removeId = disconnectedAgent.UUID
	}
	channelsMux.Lock()
	removeChannel := channels[removeId]
	channelsMux.Unlock()
	if removeChannel == nil {
		log.Println("Trying to close a channel with id: " + removeId + ", but channel does not exist. Returning")
		return
	}
	removeChannel.Close = true

	removeChannel.sub.Cancel()
	removeChannel.topic.Close()
	channelsMux.Lock()
	delete(channels, removeId)
	channelsMux.Unlock()
}

// readLoop pulls messages from the pubsub topic and pushes them onto the Messages channel.
func (cr *Channel) readLoop() {
	for {

		msg, err := cr.sub.Next(cr.ctx)
		if err != nil {
			close(cr.Messages)
			return
		}
		// only forward messages delivered by others
		if msg.ReceivedFrom == cr.self {
			continue
		}
		var cm Message

		switch cr.roomType {

		case StateMessageType:

			cm = new(StateMessage)
			break
		case MissionMessageType:

			cm = new(MissionMessage)
			break
		case GoalMessageType:

			cm = new(GoalMessage)
			break
		default:
			cm = new(DiscoveryMessage)
		}
		//cm := new(Message)
		err = json.Unmarshal(msg.Data, cm)
		if err != nil {
			log.Println("eeerrrr")
			log.Println(err)
			//log.Println(msg)
			continue
		}
		//log.Println("messages received")
		// send valid messages onto the Messages channel
		//fmt.Println(reflect.TypeOf(cm).String())
		//fmt.Println(cm)
		cr.Messages <- &cm
	}
}
