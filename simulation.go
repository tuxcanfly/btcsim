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
	"fmt"
	"log"
	"os"

	"github.com/btcsuite/btcd/wire"
	rpc "github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"
)

// MissingCertPairFile is raised when one of the cert pair files is missing
type MissingCertPairFile string

func (m MissingCertPairFile) Error() string {
	return fmt.Sprintf("could not find TLS certificate pair file: %v", m)
}

// Simulation contains the data required to run a simulation
type Simulation struct {
	com    *Communication
	actors []*Actor
}

// NewSimulation returns a Simulation instance
func NewSimulation() *Simulation {
	s := &Simulation{
		actors: make([]*Actor, 0, *numActors),
		com:    NewCommunication(),
	}
	return s
}

// Start runs the simulation by launching a node, actors and com.Start
// which communicates with the actors. It waits until the simulation
// finishes or is interrupted
func (s *Simulation) Start() error {

	// re-use existing cert, key if both are present
	// if only one of cert, key is missing, exit with err message
	haveCert := fileExists(CertFile)
	haveKey := fileExists(KeyFile)
	switch {
	case haveCert && !haveKey:
		return MissingCertPairFile(KeyFile)
	case !haveCert && haveKey:
		return MissingCertPairFile(CertFile)
	case !haveCert:
		// generate new cert pair if both cert and key are missing
		err := genCertPair(CertFile, KeyFile)
		if err != nil {
			return err
		}
	}

	// Setup node handlers to queue block processing
	ntfnHandlers := &rpc.NotificationHandlers{
		OnBlockConnected: func(hash *wire.ShaHash, height int32) {
			block := &Block{
				hash:   hash,
				height: height,
			}
			select {
			case s.com.blockQueue.enqueue <- block:
			case <-s.com.exit:
			}
		},
	}

	log.Println("Starting node on simnet...")

	// Initialize and setup the main btcd node args
	args, err := newBtcdArgs("node")
	if err != nil {
		return err
	}

	// Initialize data and log dirs
	if err := args.SetDefaults(); err != nil {
		return err
	}
	defer args.Cleanup()

	// Initialize the main node and the client
	node, err := NewNodeFromArgs(args, ntfnHandlers, nil)
	if err != nil {
		return err
	}
	if err := node.Start(); err != nil {
		return err
	}
	if err := node.Connect(); err != nil {
		return err
	}
	if err := node.client.NotifyBlocks(); err != nil {
		return err
	}
	if err := node.client.NotifyNewTransactions(false); err != nil {
		return err
	}

	// Initialize the actors
	for i := 0; i < *numActors; i++ {
		a, err := NewActor(node, uint16(18557+i))
		if err != nil {
			return err
		}
		s.actors = append(s.actors, a)
	}

	// Start actors
	for _, a := range s.actors {
		s.com.wg.Add(1)
		go func(a *Actor, com *Communication) {
			defer s.com.wg.Done()
			if err := a.Start(os.Stderr, os.Stdout, com); err != nil {
				s.com.errs <- err
				return
			}
			defer a.Shutdown()
		}(a, s.com)
	}

	miningAddrs := make([]btcutil.Address, len(s.actors))
	for i, a := range s.actors {
		select {
		case miningAddrs[i] = <-a.miningAddr:
		case <-a.quit:
			// This actor has quit
			select {
			case <-s.com.exit:
				return nil
			default:
			}
		}
	}

	// Start a miner process.
	miner, err := NewMiner(miningAddrs)
	if err != nil {
		return err
	}

	// Add mining node listen interface as a node
	node.client.AddNode("localhost:18550", rpc.ANAdd)

	s.com.Start(miner, s.actors, node)

	// if we receive an interrupt, proceed to shutdown
	addInterruptHandler(func() {
		close(s.com.exit)
	})

	<-s.com.exit
	return nil
}
