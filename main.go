package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli"

	bitswap "github.com/ipfs/go-bitswap"
	bsnet "github.com/ipfs/go-bitswap/network"
	bserv "github.com/ipfs/go-blockservice"
	car "github.com/ipfs/go-car"
	cid "github.com/ipfs/go-cid"
	dstore "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	dag "github.com/ipfs/go-merkledag"
	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-host"
	inet "github.com/libp2p/go-libp2p-net"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	routing "github.com/libp2p/go-libp2p-routing-helpers"
	ma "github.com/multiformats/go-multiaddr"
	address "github.com/whyrusleeping/f2/address"
)

func handleIncomingBlocks(ctx context.Context, bsub *pubsub.Subscription, s *Syncer) {
	for {
		msg, err := bsub.Next(ctx)
		if err != nil {
			fmt.Println("error from block subscription: ", err)
			continue
		}

		blk, err := DecodeBlockMsg(msg.GetData())
		if err != nil {
			log.Error("got invalid block over pubsub: ", err)
			continue
		}

		go func() {
			msgs, err := s.bsync.FetchMessagesByCids(blk.Messages)
			if err != nil {
				log.Errorf("failed to fetch all messages for block received over pubusb: %s", err)
				return
			}
			fmt.Println("inform new block over pubsub")
			s.InformNewBlock(msg.GetFrom(), &FullBlock{
				Header:   blk.Header,
				Messages: msgs,
			})
		}()
	}
}

func handleIncomingMessages(ctx context.Context, mpool *MessagePool, msub *pubsub.Subscription) {
	for {
		msg, err := msub.Next(ctx)
		if err != nil {
			fmt.Println("error from message subscription: ", err)
			continue
		}

		m, err := DecodeSignedMessage(msg.GetData())
		if err != nil {
			log.Errorf("got incorrectly formatted Message: %s", err)
			continue
		}

		if err := mpool.Add(m); err != nil {
			log.Errorf("failed to add message from network to message pool: %s", err)
			continue
		}
	}
}

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "mine",
		},
		cli.StringFlag{
			Name: "bootstrap",
		},
		cli.StringFlag{
			Name: "genesis",
		},
	}
	app.Action = func(c *cli.Context) error {
		ctx := context.Background()

		ds := dsync.MutexWrap(dstore.NewMapDatastore())
		bs := bstore.NewBlockstore(dsync.MutexWrap(dstore.NewMapDatastore()))
		bs = bstore.NewIdStore(bs)
		w := NewWallet()

		var genb *GenesisBootstrap
		genfile := c.String("genesis")
		if genfile == "" {
			var err error
			genb, err = MakeGenesisBlock(bs, w)
			if err != nil {
				return err
			}

			if err := WriteGenesisCar(bs, genb.Genesis); err != nil {
				return err
			}
		} else {
			gb, err := LoadGenesisFromCar(bs, genfile)
			if err != nil {
				return err
			}

			genb = &GenesisBootstrap{
				Genesis: gb,
			}
		}

		bundle, err := setupNode(ctx, ds, bs, w, genb.Genesis)
		if err != nil {
			return err
		}

		for _, a := range bundle.host.Addrs() {
			fmt.Printf("%s/ipfs/%s\n", a, bundle.host.ID())
		}

		if c.Bool("mine") {
			var nonce uint64

			doAFlip := func() {
				var kt string
				if nonce%2 == 0 {
					kt = KTBLS
				} else {
					kt = KTSecp256k1
				}

				naddr, err := w.GenerateKey(kt)
				if err != nil {
					panic(err)
				}

				msg := &Message{
					Nonce:    nonce,
					To:       naddr,
					From:     genb.MinerKey,
					Value:    NewInt(50),
					GasLimit: NewInt(0),
					GasPrice: NewInt(0),
				}
				nonce++

				smsg, err := SignMessage(w, msg)
				if err != nil {
					panic(err)
				}

				if err := bundle.mpool.Add(smsg); err != nil {
					panic(err)
				}
			}

			doAFlip()

			bundle.syncer.syncMode = CaughtUp
			m := NewMiner(bundle.chainstore, genb.MinerKey, bundle.mpool, func(b *FullBlock) {
				fmt.Println("mined a new block!", b.Cid())
				mcids := make([]cid.Cid, 0, len(b.Messages))
				for _, m := range b.Messages {
					mcids = append(mcids, m.Cid())
				}

				bmsg := &BlockMsg{
					Header:   b.Header,
					Messages: mcids,
				}
				bdata, err := bmsg.Serialize()
				if err != nil {
					log.Errorf("failed to serialize newly mined block: %s", err)
					return
				}
				bundle.pubsub.Publish("/fil/blocks", bdata)

				for i := 0; i < 9; i++ {
					doAFlip()
				}
			})
			go m.Mine(context.Background())
			fmt.Println("mining...")
		}

		bootstrap := c.String("bootstrap")
		if bootstrap != "" {
			bundle.syncer.syncMode = Bootstrap
			maddr, err := ma.NewMultiaddr(bootstrap)
			if err != nil {
				return err
			}

			pinfo, err := pstore.InfoFromP2pAddr(maddr)
			if err != nil {
				return err
			}

			if err := bundle.host.Connect(ctx, *pinfo); err != nil {
				return err
			}
			fmt.Println("connected successfully!")
		}

		select {}
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}

type Bundle struct {
	host       host.Host
	pubsub     *pubsub.PubSub
	bstore     bstore.Blockstore
	dstore     dstore.Datastore
	syncer     *Syncer
	chainstore *ChainStore
	wallet     *Wallet
	mpool      *MessagePool

	minerAddr address.Address
}

func setupNode(ctx context.Context, ds dstore.Datastore, bs bstore.Blockstore, w *Wallet, gen *BlockHeader) (*Bundle, error) {

	fmt.Println("setup node")

	cs := NewChainStore(bs, ds)
	if err := cs.SetGenesis(gen); err != nil {
		panic(err)
	}

	mpool := NewMessagePool(cs)

	h, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}

	bsnet := bsnet.NewFromIpfsHost(h, routing.Null{})
	bswap := bitswap.New(ctx, bsnet, bs)

	bsync := NewBlockSyncClient(bswap.(*bitswap.Bitswap), h.NewStream)

	bss := NewBlockSyncService(cs)
	h.SetStreamHandler(BlockSyncProtocolID, bss.HandleStream)

	s, err := NewSyncer(cs, bsync)
	if err != nil {
		panic(err)
	}

	hello := NewHelloService(cs, s, h.NewStream)
	h.SetStreamHandler(HelloProtocolID, hello.HandleStream)

	bundle := inet.NotifyBundle{
		ConnectedF: func(_ inet.Network, c inet.Conn) {
			go func() {
				if err := hello.SayHello(context.Background(), c.RemotePeer()); err != nil {
					log.Error("failed to say hello: ", err)
					return
				}
			}()
		},
	}
	h.Network().Notify(&bundle)

	pubsub, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}

	blocksub, err := pubsub.Subscribe("/fil/blocks")
	if err != nil {
		panic(err)
	}

	go handleIncomingBlocks(ctx, blocksub, s)

	msgsub, err := pubsub.Subscribe("/fil/messages")
	if err != nil {
		panic(err)
	}

	go handleIncomingMessages(ctx, mpool, msgsub)

	return &Bundle{
		host:       h,
		mpool:      mpool,
		pubsub:     pubsub,
		chainstore: cs,
		syncer:     s,
		dstore:     ds,
		bstore:     bs,
		wallet:     w,
	}, nil
}

func LoadGenesisFromCar(bs bstore.Blockstore, genfile string) (*BlockHeader, error) {
	fi, err := os.Open(genfile)
	if err != nil {
		return nil, err
	}
	defer fi.Close()

	header, err := car.LoadCar(bs, fi)
	if err != nil {
		return nil, err
	}

	sb, err := bs.Get(header.Roots[0])
	if err != nil {
		return nil, err
	}

	return DecodeBlock(sb.RawData())
}

func WriteGenesisCar(bs bstore.Blockstore, gen *BlockHeader) error {
	fi, err := os.Create("genesis.car")
	if err != nil {
		return err
	}

	defer fi.Close()

	bserv := bserv.New(bs, nil)
	ds := dag.NewDAGService(bserv)

	return car.WriteCar(context.TODO(), ds, []cid.Cid{gen.Cid()}, fi)
}

func SignMessage(w *Wallet, msg *Message) (*SignedMessage, error) {
	mdata, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	sig, err := w.Sign(msg.From, mdata)
	if err != nil {
		return nil, err
	}

	return &SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}
