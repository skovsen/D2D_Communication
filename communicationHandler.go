package comm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/alexandrainst/agentlogic"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/multiformats/go-multiaddr"
)

// DiscoveryInterval is how often we re-publish our mDNS records.
const DiscoveryInterval = time.Hour

// DiscoveryServiceTag is used in our mDNS advertisements to discover other chat peers.
const DiscoveryServiceTag = "D2D_Comunidad"

var channels = make(map[string]*Channel)

var channelsMux = &sync.Mutex{}

var SelfId peer.ID

var ps *pubsub.PubSub

//vars for registration
var myType agentlogic.AgentType
var discoveryPath string
var statePath string
var reorganizationPath string
var recalculationPath string

var DiscoveryChannel = make(chan *DiscoveryMessage, BufferSize)
var StateChannel = make(chan *StateMessage, BufferSize)
var ReorganizationChannel = make(chan *DiscoveryMessage, BufferSize)
var RecalculationChannel = make(chan *DiscoveryMessage, BufferSize)
var MissionChannel = make(chan *MissionMessage, BufferSize)
var GoalChannel = make(chan *GoalMessage, BufferSize)

func InitD2DCommuncation(agentType agentlogic.AgentType) {
	myType = agentType
	ctx := context.Background()

	// Create a new libp2p host
	h, err := createHost()
	if err != nil {
		panic(err)
	}

	fmt.Println("Host id is ", h.ID())
	SelfId = h.ID()

	// create a new PubSub service using the GossipSub router
	ps, err = pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}

	// setup local mDNS discovery
	err = setupDiscovery(ctx, h)
	if err != nil {
		panic(err)
	}
}

func InitCommunicationType(path string, messageType MessageType) {
	//log.Println("PATH: " + path)
	channelsMux.Lock()
	if channels[path] != nil {
		//channel already created - ignoring
		//log.Println("channel with path: " + path + " already in list. returning")
		channelsMux.Unlock()
		return
	}
	channelsMux.Unlock()
	switch messageType {
	case StateMessageType:
		statePath = path
		break
	case DiscoveryMessageType:
		discoveryPath = path
		break
	case ReorganizationMessageType:
		reorganizationPath = path
		break
	case RecalculatorMessageType:
		recalculationPath = path
		break
	}
	ctx := context.Background()
	cr, err := JoinPath(ctx, ps, SelfId, path, messageType)
	if err != nil {
		panic(err)
	}
	go cr.readMessages()
}

func SendMission(senderId string, mission *agentlogic.Mission, channelPath string) error {
	channelsMux.Lock()
	channel := channels[channelPath]
	channelsMux.Unlock()

	m := MissionMessage{
		MessageMeta: MessageMeta{MsgType: MissionMessageType, SenderId: senderId, SenderType: myType},
		Content:     *mission,
	}
	if senderId == channelPath {
		MissionChannel <- &m
		return nil
	}
	if channel == nil {

		return errors.New("channel not found with path: " + channelPath)
	}
	msgBytes, err := json.Marshal(m)

	if err != nil {
		return err
	}
	channel.topic.Publish(channel.ctx, msgBytes)

	return nil
}

func SendGoalFound(senderId string, goal agentlogic.Goal, posiition agentlogic.Vector, channelPath string) error {
	channelsMux.Lock()
	channel := channels[channelPath]
	channelsMux.Unlock()
	if channel == nil {
		return errors.New("channel not found with path: " + channelPath)
	}
	m := GoalMessage{
		MessageMeta: MessageMeta{MsgType: GoalMessageType, SenderId: senderId, SenderType: myType},
		Content:     goal,
		Position:    posiition,
	}

	if senderId == channelPath {
		GoalChannel <- &m
	}
	msgBytes, err := json.Marshal(m)

	if err != nil {
		return err
	}
	channel.topic.Publish(channel.ctx, msgBytes)

	return nil
}

func SendState(state *agentlogic.State) {
	//log.Println("Stating me")
	//state channel
	channelsMux.Lock()
	channel := channels[statePath]
	channelsMux.Unlock()
	m := StateMessage{
		MessageMeta: MessageMeta{MsgType: MissionMessageType, SenderId: state.ID, SenderType: myType},
		Content:     *state,
	}
	// log.Println("state msg:")
	// log.Println(m)
	msgBytes, err := json.Marshal(m)

	if err != nil {
		panic(err)
	}
	channel.topic.Publish(channel.ctx, msgBytes)

}

func AnnounceSelf(metadata *agentlogic.Agent) {
	//log.Println("Announce:")
	// log.Println(*metadata)
	//registration channel
	channelsMux.Lock()
	channel := channels[discoveryPath]
	channelsMux.Unlock()
	m := DiscoveryMessage{
		MessageMeta: MessageMeta{MsgType: MissionMessageType, SenderId: metadata.UUID, SenderType: myType},
		Content:     *metadata,
	}

	msgBytes, err := json.Marshal(m)

	if err != nil {
		panic(err)
	}
	channel.topic.Publish(channel.ctx, msgBytes)
}

func SendReorganization(metadata agentlogic.Agent, selfId string) {
	log.Println("from: " + selfId + " about:" + metadata.UUID)
	//registration channel
	channelsMux.Lock()
	channel := channels[reorganizationPath]
	channelsMux.Unlock()
	m := DiscoveryMessage{
		MessageMeta: MessageMeta{MsgType: ReorganizationMessageType, SenderId: selfId, SenderType: myType},
		Content:     metadata,
	}
	//log.Println(m)
	msgBytes, err := json.Marshal(m)

	if err != nil {
		panic(err)
	}
	channel.topic.Publish(channel.ctx, msgBytes)
}

func SendRecalculation(metadata agentlogic.Agent, selfId string) {
	log.Println(selfId + ": send recalc message on " + recalculationPath)

	//registration channel
	channelsMux.Lock()
	channel := channels[recalculationPath]
	channelsMux.Unlock()
	m := DiscoveryMessage{
		MessageMeta: MessageMeta{MsgType: RecalculatorMessageType, SenderId: selfId, SenderType: myType},
		Content:     metadata,
	}

	msgBytes, err := json.Marshal(m)

	if err != nil {
		panic(err)
	}
	channel.topic.Publish(channel.ctx, msgBytes)
}

func createHost() (host.Host, error) {
	// Creates a new RSA key pair for this host.
	var r = rand.Reader
	prvKey, _, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	if err != nil {
		panic(err)
	}

	//host will listens on a random local TCP port on the IPv4.
	sourceMultiAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/0"))

	// libp2p.New constructs a new libp2p Host.
	host, err := libp2p.New(
		context.Background(),
		libp2p.ListenAddrs(sourceMultiAddr),
		libp2p.Identity(prvKey),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
	)

	if err != nil {
		return nil, err
	}

	return host, nil
}

func (cr *Channel) readMessages() {
	//log.Println("Starting cr.readMessages() with path: " + cr.path)
	for {
		message := <-cr.Messages
		if cr.Close {
			log.Println("closing channel: " + cr.path)
			return
		}
		switch cr.roomType {
		case DiscoveryMessageType:
			msg := (*message).(*DiscoveryMessage)
			DiscoveryChannel <- msg
			break
		case StateMessageType:
			msg := (*message).(*StateMessage)
			StateChannel <- msg
			break
		case ReorganizationMessageType:
			msg := (*message).(*DiscoveryMessage)
			ReorganizationChannel <- msg
			break
		case MissionMessageType:
			msg := (*message).(*MissionMessage)
			MissionChannel <- msg
			break
		case RecalculatorMessageType:
			msg := (*message).(*DiscoveryMessage)
			RecalculationChannel <- msg
			break
		case GoalMessageType:
			msg := (*message).(*GoalMessage)
			GoalChannel <- msg
		}
	}
}

// discoveryNotifee gets notified when we find a new peer via mDNS discovery
type discoveryNotifee struct {
	h host.Host
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're connected,
// the PubSub system will automatically start interacting with them if they also
// support PubSub.
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	//fmt.Printf("discovered new peer %s\n", pi.ID.Pretty())
	err := n.h.Connect(context.Background(), pi)
	if err != nil {
		fmt.Printf("error connecting to peer %s: %s\n", pi.ID.Pretty(), err)
	}
}

// setupDiscovery creates an mDNS discovery service and attaches it to the libp2p Host.
// This lets us automatically discover peers on the same LAN and connect to them.
func setupDiscovery(ctx context.Context, h host.Host) error {
	// setup mDNS discovery to find local peers
	disc, err := discovery.NewMdnsService(ctx, h, DiscoveryInterval, DiscoveryServiceTag)
	if err != nil {
		return err
	}

	n := discoveryNotifee{h: h}
	disc.RegisterNotifee(&n)
	return nil
}
