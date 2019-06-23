package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	dstore "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	address "github.com/whyrusleeping/f2/address"
)

var _ = fmt.Errorf
var _ = time.April

func randomMessage(w *Wallet, mpool *MessagePool, from address.Address, nonce uint64) {
	naddr, err := w.GenerateKey(KTSecp256k1)
	if err != nil {
		panic(err)
	}

	msg := &Message{
		Nonce:    nonce,
		To:       naddr,
		From:     from,
		Value:    NewInt(50),
		GasLimit: NewInt(0),
		GasPrice: NewInt(0),
	}
	nonce++

	smsg, err := SignMessage(w, msg)
	if err != nil {
		panic(err)
	}

	if err := mpool.Add(smsg); err != nil {
		panic(err)
	}
}

func TestChainCreation(t *testing.T) {
	ctx := context.Background()

	ds := dsync.MutexWrap(dstore.NewMapDatastore())
	bs := bstore.NewBlockstore(dsync.MutexWrap(dstore.NewMapDatastore()))
	bs = bstore.NewIdStore(bs)
	w := NewWallet()

	genb, err := MakeGenesisBlock(bs, w)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := setupNode(ctx, ds, bs, w, genb.Genesis)
	if err != nil {
		t.Fatal(err)
	}

	m := NewMiner(bundle.chainstore, genb.MinerKey, bundle.mpool, func(b *FullBlock) {})
	m.Delay = 0

	randomMessage(w, bundle.mpool, genb.MinerKey, 0)
	randomMessage(w, bundle.mpool, genb.MinerKey, 1)
	randomMessage(w, bundle.mpool, genb.MinerKey, 2)

	base := m.GetBestMiningCandidate()
	b, err := m.mineOne(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	fts := NewFullTipSet([]*FullBlock{b})

	if err := bundle.syncer.SyncCaughtUp(fts); err != nil {
		t.Fatal(err)
	}

	base = m.GetBestMiningCandidate()
	b, err = m.mineOne(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

}

func TestChainSync(t *testing.T) {
	ctx := context.Background()

	ds := dsync.MutexWrap(dstore.NewMapDatastore())
	bs := bstore.NewBlockstore(dsync.MutexWrap(dstore.NewMapDatastore()))
	bs = bstore.NewIdStore(bs)
	w := NewWallet()

	genb, err := MakeGenesisBlock(bs, w)
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := setupNode(ctx, ds, bs, w, genb.Genesis)
	if err != nil {
		t.Fatal(err)
	}

	m := NewMiner(bundle.chainstore, genb.MinerKey, bundle.mpool, func(b *FullBlock) {})
	m.Delay = 0

	nonce := uint64(0)
	for i := 0; i < 100; i++ {
		for j := 0; j < 3; j++ {
			randomMessage(w, bundle.mpool, genb.MinerKey, nonce)
			nonce++
		}

		base := m.GetBestMiningCandidate()
		b, err := m.mineOne(ctx, base)
		if err != nil {
			t.Fatal(err)
		}

		fts := NewFullTipSet([]*FullBlock{b})
		if err := bundle.syncer.SyncCaughtUp(fts); err != nil {
			t.Fatal(err)
		}
	}

	ds2 := dsync.MutexWrap(dstore.NewMapDatastore())
	bs2 := bstore.NewBlockstore(dsync.MutexWrap(dstore.NewMapDatastore()))
	bs2 = bstore.NewIdStore(bs)
	w2 := NewWallet()

	bundle2, err := setupNode(ctx, ds2, bs2, w2, genb.Genesis)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	bundle2.chainstore.headChange = func(rev, app []*TipSet) error {
		close(done)
		return nil
	}

	pi := pstore.PeerInfo{
		ID:    bundle.host.ID(),
		Addrs: bundle.host.Addrs(),
	}
	if err := bundle2.host.Connect(ctx, pi); err != nil {
		t.Fatal(err)
	}

	<-done

	best := bundle2.chainstore.GetHeaviestTipSet()
	minerBest := bundle.chainstore.GetHeaviestTipSet()
	if !best.Equals(minerBest) {
		t.Fatal("sync failed")
	}

}
