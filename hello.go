package main

import (
	"bufio"
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/protocol"
	inet "github.com/libp2p/go-libp2p-net"
	"github.com/libp2p/go-libp2p-peer"
)

const HelloProtocolID = "/fil/hello/1.0.0"

func init() {
	cbor.RegisterCborType(HelloMessage{})
}

type HelloMessage struct {
	HeaviestTipSet       []cid.Cid
	HeaviestTipSetWeight uint64
	GenesisHash          cid.Cid
}

type NewStreamFunc func(context.Context, peer.ID, ...protocol.ID) (inet.Stream, error)

type HelloService struct {
	cs        *ChainStore
	newStream NewStreamFunc
	syncer    *Syncer
}

func NewHelloService(cs *ChainStore, sync *Syncer, nstream NewStreamFunc) *HelloService {
	return &HelloService{
		cs:        cs,
		syncer:    sync,
		newStream: nstream,
	}
}

func (hs *HelloService) HandleStream(s inet.Stream) {
	defer s.Close()

	fmt.Println("HANDLIng HELLO STREAM")

	var hmsg HelloMessage
	if err := ReadCborRPC(bufio.NewReader(s), &hmsg); err != nil {
		log.Error("failed to read hello message: ", err)
		return
	}
	log.Errorf("heaviest tipset from message: ", hmsg.HeaviestTipSet)
	log.Errorf("got genesis from hello: ", hmsg.GenesisHash)

	if hmsg.GenesisHash != hs.syncer.genesis.Cids()[0] {
		log.Error("other peer has different genesis!")
		s.Conn().Close()
		return
	}

	ts, err := hs.syncer.FetchTipSet(context.Background(), s.Conn().RemotePeer(), hmsg.HeaviestTipSet)
	if err != nil {
		log.Errorf("failed to fetch tipset from peer during hello: %s", err)
		return
	}

	hs.syncer.InformNewHead(s.Conn().RemotePeer(), ts)
}

func (hs *HelloService) SayHello(ctx context.Context, pid peer.ID) error {
	s, err := hs.newStream(ctx, pid, HelloProtocolID)
	if err != nil {
		return err
	}

	hts := hs.cs.GetHeaviestTipSet()
	weight := hs.cs.Weight(hts)
	gen, err := hs.cs.GetGenesis()
	if err != nil {
		return err
	}

	hmsg := &HelloMessage{
		HeaviestTipSet:       hts.Cids(),
		HeaviestTipSetWeight: weight,
		GenesisHash:          gen.Cid(),
	}
	fmt.Println("SENDING HELLO MESSAGE: ", hts.Cids())
	fmt.Println("hello message genesis: ", gen.Cid())

	if err := WriteCborRPC(s, hmsg); err != nil {
		return err
	}

	return nil
}
