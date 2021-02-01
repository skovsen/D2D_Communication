package comm

import (
	"context"
	"encoding/json"
	"log"

	"github.com/alexandrainst/agentlogic"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/paulmach/orb"
)

type VisualizationChannel struct {
	Messages chan *VisualizationMessage
	ctx      context.Context
	ps       *pubsub.PubSub
	topic    *pubsub.Topic
	sub      *pubsub.Subscription
	roomType MessageType
	path     string
	self     peer.ID
}

type VisualizationMessage struct {
	MsgType          MessageType
	ContentType      MessageType
	DiscoveryMessage agentlogic.Agent
	StateMessage     agentlogic.State
	MissionMessage   agentlogic.Mission
	GoalMessage      GoalMessage
	SenderId         string
	SenderType       agentlogic.AgentType
	MissionBound     orb.Bound
}

// type VisGoalMessage struct {
// 	AgentId  string
// 	MsgType  MessageType
// 	Position agentlogic.Vector
// 	Poi      string
// }

const VisualizationMessageType = -13
const VisualizationAgentType = -133
const visPath = "D2D_visualization"

var ChannelVisualization = make(chan Message, BufferSize)
var visChannel *VisualizationChannel

func GetVizChannel() *VisualizationChannel {
	return visChannel
}

func sendingVisualizaMessage(channel *VisualizationChannel) {

	go func() {
		for {
			msg := <-ChannelVisualization

			vm := VisualizationMessage{
				MsgType: VisualizationMessageType,
			}

			switch msg.(type) {
			case *StateMessage:
				assertedMessage := *msg.(*StateMessage)
				vm.ContentType = assertedMessage.MessageMeta.MsgType
				vm.SenderId = assertedMessage.MessageMeta.SenderId
				vm.StateMessage = assertedMessage.Content
				vm.SenderType = assertedMessage.MessageMeta.SenderType
			case *MissionMessage:
				assertedMessage := *msg.(*MissionMessage)
				vm.ContentType = assertedMessage.MessageMeta.MsgType
				vm.SenderId = assertedMessage.MessageMeta.SenderId
				vm.MissionMessage = assertedMessage.Content
				vm.SenderType = assertedMessage.MessageMeta.SenderType
			case *DiscoveryMessage:
				assertedMessage := *msg.(*DiscoveryMessage)
				vm.ContentType = assertedMessage.MessageMeta.MsgType
				vm.SenderId = assertedMessage.MessageMeta.SenderId
				vm.DiscoveryMessage = assertedMessage.Content
				vm.SenderType = assertedMessage.MessageMeta.SenderType
				if vm.ContentType == RecalculatorMessageType {
					log.Println("VIZ: from: " + vm.SenderId + " about: " + vm.DiscoveryMessage.UUID)
				}

			default:
				log.Println("unknown message")
				continue
			}

			msgBytes, err := json.Marshal(vm)

			if err != nil {
				panic(err)
			}
			channel.topic.Publish(channel.ctx, msgBytes)
		}
	}()
}

func InitVisualizationMessages(subscribe bool) *VisualizationChannel {
	log.Println("Start Visualization communication")
	ctx := context.Background()
	ch, err := joinVisualizationComm(ctx, ps, SelfId, visPath, VisualizationMessageType, subscribe)
	if err != nil {
		panic(err)
	}

	sendingVisualizaMessage(ch)

	return ch
}

func SendVizGoal(goal GoalMessage) {
	vm := VisualizationMessage{
		MsgType:     VisualizationMessageType,
		ContentType: GoalMessageType,
		GoalMessage: goal,
		SenderType:  VisualizationAgentType,
	}

	msgBytes, err := json.Marshal(vm)

	if err != nil {
		panic(err)
	}
	visChannel.topic.Publish(visChannel.ctx, msgBytes)

}

func joinVisualizationComm(ctx context.Context, ps *pubsub.PubSub, selfID peer.ID, path string, roomType MessageType, subscribePath bool) (*VisualizationChannel, error) {

	topic, err := ps.Join(path)
	if err != nil {
		return nil, err
	}

	// and subscribe to it
	sub, err := topic.Subscribe()
	if err != nil {
		return nil, err
	}

	ch := &VisualizationChannel{

		ctx:      ctx,
		ps:       ps,
		topic:    topic,
		sub:      sub,
		self:     selfID,
		path:     path,
		roomType: roomType,

		Messages: make(chan *VisualizationMessage, BufferSize),
	}
	visChannel = ch
	// start reading messages from the subscription in a loop
	log.Println("Channel with path " + path + " joined")
	if subscribePath {
		go ch.readLoop()
		log.Println("read loop started")
	}

	return ch, nil
}

func (vc *VisualizationChannel) readLoop() {
	for {
		msg, err := vc.sub.Next(vc.ctx)
		if err != nil {
			close(vc.Messages)
			return
		}

		// only forward messages delivered by others
		if msg.ReceivedFrom == vc.self {
			continue
		}
		vm := new(VisualizationMessage)

		err = json.Unmarshal(msg.Data, vm)
		if err != nil {
			log.Println("eeerrrr!!!!")
			log.Println(err)
			//log.Println(msg.Data)
			continue
		}
		//log.Println("messages received")
		// send valid messages onto the Messages channel
		vc.Messages <- vm
	}
}
