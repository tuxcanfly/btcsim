// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"

	rpc "github.com/conformal/btcrpcclient"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// Miner holds all the core features required to register, run, control,
// and kill a cpu-mining btcd instance.
type Miner struct {
	*Node
}

// NewMiner starts a cpu-mining enabled btcd instane and returns an rpc client
// to control it.
func NewMiner(miningAddrs []btcutil.Address, exit chan struct{},
	height chan<- int32, txpool chan<- struct{}) (*Miner, error) {

	ntfnHandlers := &rpc.NotificationHandlers{
		// When a block higher than stopBlock connects to the chain,
		// send a signal to stop actors. This is used so main can break from
		// select and call actor.Stop to stop actors.
		OnBlockConnected: func(hash *btcwire.ShaHash, h int32) {
			if h >= int32(*startBlock)-1 {
				if height != nil {
					height <- h
				}
			} else {
				fmt.Printf("\r%d/%d", h, *startBlock)
			}
		},
		// Send a signal that a tx has been accepted into the mempool. Based on
		// the tx curve, the receiver will need to wait until required no of tx
		// are filled up in the mempool
		OnTxAccepted: func(hash *btcwire.ShaHash, amount btcutil.Amount) {
			if txpool != nil {
				// this will not be blocked because we're creating only
				// required no of tx and receiving all of them
				txpool <- struct{}{}
			}
		},
	}

	log.Println("Starting miner on simnet...")
	args, err := newBitcoindArgs("miner")
	if err != nil {
		return nil, err
	}

	// set miner args - it listens on a different port
	// because a node is already running on the default port
	args.Listen = "127.0.0.1"
	args.Port = "18550"
	args.RPCListen = "127.0.0.1"
	args.RPCPort = "18551"
	// if passed, set blockmaxsize to allow mining large blocks
	args.Extra = []string{fmt.Sprintf("--blockmaxsize=%d", *maxBlockSize)}
	// set the actors' mining addresses
	for _, addr := range miningAddrs {
		// make sure addr was initialized
		if addr != nil {
			args.Extra = append(args.Extra, "--miningaddr="+addr.EncodeAddress())
		}
	}
	// Add node as peer for mining
	args.Extra = append(args.Extra, "--addnode=127.0.0.1:18555")
	args.Extra = append(args.Extra, "--blocknotify=btcnotifier --name=miner.block --event=block --port=19550 %s")
	args.Extra = append(args.Extra, "--walletnotify=btcnotifier --name=miner.tx --event=tx --port=19551 %s")

	logFile, err := getLogFile(args.prefix)
	if err != nil {
		log.Printf("Cannot get log file, logging disabled: %v", err)
	}
	node, err := NewNodeFromArgs(args, nil, logFile)

	miner := &Miner{
		Node: node,
	}
	if err := node.Start(); err != nil {
		log.Printf("%s: Cannot start mining node: %v", miner, err)
		return nil, err
	}
	if err := node.Connect(); err != nil {
		log.Printf("%s: Cannot connect to node: %v", miner, err)
		return nil, err
	}

	go func() {
		rpcListener := NewListener("miner.block", "19550", ntfnHandlers)
		if err := rpcListener.listen(); err != nil {
			log.Printf("err: %v", err)
		}
	}()
	go func() {
		rpcListener := NewListener("miner.tx", "19551", ntfnHandlers)
		if err := rpcListener.listen(); err != nil {
			log.Printf("err: %v", err)
		}
	}()

	// Use just one core for mining.
	if err := miner.StartMining(); err != nil {
		return miner, err
	}

	log.Printf("%s: Generating %v blocks...", miner, *startBlock)
	return miner, nil
}

// StartMining sets the cpu miner to mine coins
func (m *Miner) StartMining() error {
	if err := m.client.SetGenerate(true, 1); err != nil {
		log.Printf("%s: Cannot start mining: %v", m, err)
		return err
	}
	return nil
}

// StopMining stops the cpu miner from mining coins
func (m *Miner) StopMining() error {
	if err := m.client.SetGenerate(false, 0); err != nil {
		log.Printf("%s: Cannot stop mining: %v", m, err)
		return err
	}
	return nil
}
