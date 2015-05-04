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
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	rpc "github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"
)

// Block contains the block hash and height as received in a
// OnBlockConnected notification
type Block struct {
	hash   *wire.ShaHash
	height int32
}

// blockQueue is a queue of blocks received from OnBlockConnected
// waiting to be processed
type blockQueue struct {
	enqueue   chan *Block
	dequeue   chan *Block
	processed chan *Block
}

// Communication is consisted of the necessary primitives used
// for communication between the main goroutine and actors.
type Communication struct {
	wg            sync.WaitGroup
	downstream    chan btcutil.Address
	timeReceived  chan time.Time
	blockTxCount  chan int
	exit          chan struct{}
	errs          chan error
	height        chan int32
	split         chan int
	coinbaseQueue chan *btcutil.Tx
	blocks        *blockQueue
}

// NewCommunication creates a new data structure with all the
// necessary primitives for a fully functional simulation to
// happen.
func NewCommunication() *Communication {
	return &Communication{
		downstream:    make(chan btcutil.Address, *numActors),
		timeReceived:  make(chan time.Time, *numActors),
		blockTxCount:  make(chan int, *numActors),
		height:        make(chan int32),
		split:         make(chan int),
		coinbaseQueue: make(chan *btcutil.Tx, blockchain.CoinbaseMaturity),
		exit:          make(chan struct{}),
		errs:          make(chan error, *numActors),
		blocks: &blockQueue{
			enqueue:   make(chan *Block),
			dequeue:   make(chan *Block),
			processed: make(chan *Block),
		},
	}
}

// Start handles the main part of a simulation by starting
// all the necessary goroutines.
func (com *Communication) Start(actors []*Actor, node *Node) {
	// Start a goroutine to coordinate transactions
	com.wg.Add(1)
	go com.Communicate(node, actors)

	com.wg.Add(1)
	go com.queueBlocks()

	com.wg.Add(1)
	go com.poolUtxos(node.client, actors)

	return
}

// queueBlocks queues blocks in the order they are received
func (com *Communication) queueBlocks() {
	defer com.wg.Done()

	var blocks []*Block
	enqueue := com.blocks.enqueue
	var dequeue chan *Block
	var next *Block
out:
	for {
		select {
		case n, ok := <-enqueue:
			if !ok {
				// If no blocks are queued for handling,
				// the queue is finished.
				if len(blocks) == 0 {
					break out
				}
				// nil channel so no more reads can occur.
				enqueue = nil
				continue
			}
			if len(blocks) == 0 {
				next = n
				dequeue = com.blocks.dequeue
			}
			blocks = append(blocks, n)
		case dequeue <- next:
			blocks[0] = nil
			blocks = blocks[1:]
			if len(blocks) != 0 {
				next = blocks[0]
			} else {
				// If no more blocks can be enqueued, the
				// queue is finished.
				if enqueue == nil {
					break out
				}
				dequeue = nil
			}
		case <-com.exit:
			break out
		}
	}
	close(com.blocks.dequeue)
}

// poolUtxos receives a new block notification from the node server
// and pools the newly mined utxos to the corresponding actor's a.utxo
func (com *Communication) poolUtxos(client *rpc.Client, actors []*Actor) {
	defer com.wg.Done()
	// Update utxo pool on each block connected
	for {
		select {
		case b, ok := <-com.blocks.dequeue:
			if !ok {
				return
			}
			block, err := client.GetBlock(b.hash)
			if err != nil {
				log.Printf("Cannot get block: %v", err)
				return
			}
			// add new outputs to unspent pool
			for i, tx := range block.Transactions() {
			next:
				for n, vout := range tx.MsgTx().TxOut {
					if i == 0 {
						// in case of coinbase tx, add it to coinbase queue
						// if the chan is full, the first tx would be mature
						// so add it to the pool
						select {
						case com.coinbaseQueue <- tx:
							break next
						default:
							// dequeue the first mature tx
							mTx := <-com.coinbaseQueue
							// enqueue the latest tx
							com.coinbaseQueue <- tx
							// we'll process the mature tx next
							// so point tx to mTx
							tx = mTx
							// reset vout as per the new tx
							vout = tx.MsgTx().TxOut[n]
						}
					}
					// fetch actor who owns this output
					var actor *Actor
					if len(actors) == 1 {
						actor = actors[0]
					} else {
						actor, err = com.getActor(actors, vout)
						if err != nil {
							log.Printf("Cannot get actor: %v", err)
							continue next
						}
					}
					txout := com.getUtxo(tx, vout, uint32(n))
					// to be usable, the utxo amount should be
					// split-able after deducting the fee
					if txout.Amount > btcutil.Amount((*maxSplit))*(minFee) {
						// if it's usable, add utxo to actor's pool
						select {
						case actor.utxoQueue.enqueue <- txout:
						case <-com.exit:
						}
					}
				}
			}
			var txCount, utxoCount int
			for _, a := range actors {
				utxoCount += len(a.utxoQueue.utxos)
			}
			txCount = len(block.Transactions())
			log.Printf("Block %s (height %d) attached with %d transactions", b.hash, b.height, txCount)
			log.Printf("%d transaction outputs available to spend", utxoCount)
			select {
			case com.blocks.processed <- b:
			case <-com.exit:
				return
			}
			select {
			case com.blockTxCount <- txCount:
			case <-com.exit:
				return
			}
		case <-com.exit:
			return
		}
	}
}

// getActor returns the actor to which this vout belongs to
func (com *Communication) getActor(actors []*Actor,
	vout *wire.TxOut) (*Actor, error) {
	// get addrs which own this utxo
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(vout.PkScript,
		&chaincfg.SimNetParams)
	if err != nil {
		return nil, err
	}

	// we're expecting only 1 addr since we created a standard p2pkh tx
	addr := addrs[0].String()
	// find which actor this addr belongs to
	// TODO: could probably be optimized by creating
	// a global addr -> actor index rather than looking
	// up each actor addrs
	for _, actor := range actors {
		for _, actorAddr := range actor.ownedAddresses {
			if addr == actorAddr.String() {
				return actor, nil
			}
		}
	}
	err = errors.New("cannot find any actor who owns this tx output")
	return nil, err
}

// getUtxo returns a TxOut from Tx and Vout
func (com *Communication) getUtxo(tx *btcutil.Tx,
	vout *wire.TxOut, index uint32) *TxOut {
	op := wire.NewOutPoint(tx.Sha(), index)
	unspent := TxOut{
		OutPoint: op,
		Amount:   btcutil.Amount(vout.Value),
	}
	return &unspent
}

// Communicate generates tx and controls the mining according
// to the input block height vs tx count curve
func (com *Communication) Communicate(node *Node, actors []*Actor) {
	defer com.wg.Done()

	// wait until this block is processed
	select {
	case <-com.blocks.processed:
	case <-com.exit:
		return
	}

	var wg sync.WaitGroup
	// count the number of utxos available in total
	var utxoCount int
	for _, a := range actors {
		utxoCount += len(a.utxoQueue.utxos)
	}

	reqTxCount := utxoCount

	log.Printf("Generating %v transactions ...", reqTxCount)
	for i := 0; i < reqTxCount; i++ {
		fmt.Printf("\r%d/%d", i+1, reqTxCount)
		a := actors[rand.Int()%len(actors)]
		addr := a.ownedAddresses[rand.Int()%len(a.ownedAddresses)]
		select {
		case com.downstream <- addr:
		case <-com.exit:
			return
		}
	}

	fmt.Printf("\n")
	log.Printf("Waiting for miner node...")
	wg.Wait()
	// mine the above tx in the next block
	if err := node.Generate(1); err != nil {
		close(com.exit)
	}
}
