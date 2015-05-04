/*
 * Copyright (c) 2014-2015 Conformal Systems LLC <info@conformal.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"
	rpc "github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"
)

// minFee is the minimum tx fee that can be paid
const minFee btcutil.Amount = 1e4 // 0.0001 BTC

// utxoQueue is the queue of utxos belonging to a actor
// utxos are queued after a block is received and are dispatched
// to their respective owner from com.poolUtxos
// they are dequeued from simulateTx and splitUtxos
type utxoQueue struct {
	utxos   []*TxOut
	enqueue chan *TxOut
	dequeue chan *TxOut
}

// Actor describes an actor on the simulation network.  Each actor runs
// independantly without external input to decide it's behavior.
type Actor struct {
	*Node
	quit             chan struct{}
	wg               sync.WaitGroup
	ownedAddresses   []btcutil.Address
	utxoQueue        *utxoQueue
	miningAddr       chan btcutil.Address
	walletPassphrase string
}

// TxOut is a valid tx output that can be used to generate transactions
type TxOut struct {
	OutPoint *wire.OutPoint
	Amount   btcutil.Amount
}

// NewActor creates a new actor which runs its own wallet process connecting
// to the btcd node server specified by node, and listening for simulator
// websocket connections on the specified port.
func NewActor(node *Node, port uint16) (*Actor, error) {
	// Please don't run this as root.
	if port < 1024 {
		return nil, errors.New("invalid actor port")
	}

	// Set btcwallet node args
	args, err := newBtcwalletArgs(port, node.Args.(*btcdArgs))
	if err != nil {
		return nil, err
	}

	btcwallet, err := NewNodeFromArgs(args, nil, nil)
	if err != nil {
		return nil, err
	}

	a := Actor{
		Node:             btcwallet,
		quit:             make(chan struct{}),
		ownedAddresses:   make([]btcutil.Address, *maxAddresses),
		miningAddr:       make(chan btcutil.Address),
		walletPassphrase: "password",
		utxoQueue: &utxoQueue{
			enqueue: make(chan *TxOut),
			dequeue: make(chan *TxOut),
		},
	}
	return &a, nil
}

// Start creates the command to execute a wallet process and starts the
// command in the background, attaching the command's stderr and stdout
// to the passed writers. Nil writers may be used to discard output.
//
// In addition to starting the wallet process, this runs goroutines to
// handle wallet notifications and requests the wallet process to create
// an intial encrypted wallet, so that it can actually send and receive BTC.
//
// If the RPC client connection cannot be established or wallet cannot
// be created, the wallet process is killed and the actor directory
// removed.
func (a *Actor) Start(stderr, stdout io.Writer) error {
	connected := make(chan struct{})
	const timeoutSecs int64 = 3600 * 24

	if err := a.Node.Start(); err != nil {
		return err
	}
	defer a.Node.Shutdown()

	ntfnHandlers := &rpc.NotificationHandlers{
		OnClientConnected: func() {
			connected <- struct{}{}
		},
	}
	a.handlers = ntfnHandlers

	if err := a.Connect(); err != nil {
		return err
	}

	// Wait for btcd to connect
	<-connected

	// Wait for wallet sync
	for i := 0; i < *maxConnRetries; i++ {
		if _, err := a.client.GetBalance(""); err != nil {
			time.Sleep(time.Duration(i) * 50 * time.Millisecond)
			continue
		}
		break
	}

	// Create wallet addresses and unlock wallet.
	log.Printf("%s: Creating wallet addresses...", a)
	for i := range a.ownedAddresses {
		fmt.Printf("\r%d/%d", i+1, len(a.ownedAddresses))
		addr, err := a.client.GetNewAddress("")
		if err != nil {
			return err
		}
		a.ownedAddresses[i] = addr
	}
	fmt.Printf("\n")

	if err := a.client.WalletPassphrase(a.walletPassphrase,
		timeoutSecs); err != nil {
		return err
	}

	// Send a random address that will be used by the cpu miner.
	a.miningAddr <- a.ownedAddresses[rand.Int()%len(a.ownedAddresses)]

	return nil
}

// Shutdown performs a shutdown down the actor by first signalling
// all goroutines to stop, waiting for them to stop and them cleaning up
func (a *Actor) Shutdown() {
	select {
	case <-a.quit:
	default:
		close(a.quit)
		a.WaitForShutdown()
		a.Node.Shutdown()
	}
}

// WaitForShutdown waits until every actor goroutine has returned
func (a *Actor) WaitForShutdown() {
	a.wg.Wait()
}
