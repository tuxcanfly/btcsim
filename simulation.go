package main

import (
	"errors"
	"log"
	"math"
	"os"
	"time"

	rpc "github.com/conformal/btcrpcclient"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// ErrInvalidCertPair is raised in case of an error in reading the default cert pair
var ErrInvalidCertPair = errors.New("error reading cert pair")

const (
	// SimRows is the number of rows in the default curve
	SimRows = 10

	// SimUtxoCount is the starting number of utxos in the default curve
	SimUtxoCount = 2000

	// SimTxCount is the starting number of tx in the default curve
	SimTxCount = 1000
)

// Simulation contains the data required to run a simulation
type Simulation struct {
	txCurve map[int32]*Row
	com     *Communication
	actors  []*Actor
}

// NewSimulation returns a Simulation instance
func NewSimulation() *Simulation {
	s := &Simulation{
		txCurve: make(map[int32]*Row),
		actors:  make([]*Actor, 0, *numActors),
		com:     NewCommunication(),
	}
	return s
}

// readTxCurve reads and sets the txcurve to simulate
// It defaults to a simple linears simulation
func (s *Simulation) readTxCurve(txCurvePath string) error {
	var txCurve map[int32]*Row
	if txCurvePath == "" {
		// if -txcurve argument is omitted, use a simple
		// linear simulation curve as the default
		txCurve = make(map[int32]*Row, SimRows)
		for i := 1; i <= SimRows; i++ {
			block := int32(*matureBlock + i)
			txCurve[block] = &Row{
				utxoCount: i * SimUtxoCount,
				txCount:   i * SimTxCount,
			}
		}
	} else {
		file, err := os.Open(txCurvePath)
		defer file.Close()
		if err != nil {
			return err
		}
		txCurve, err = readCSV(file)
		if err != nil {
			return err
		}
	}
	s.txCurve = txCurve
	return nil
}

// updateFlags updates the flags based on the txCurve
func (s *Simulation) updateFlags() {
	// set min block number from the curve as matureBlock
	for k := range s.txCurve {
		block := int(k)
		if block < *matureBlock {
			*matureBlock = block
		}
	}

	if *maxSplit > *maxAddresses {
		// cap max split at maxaddresses, becauase each split requires
		// a unique return address
		*maxSplit = *maxAddresses
	}
	// we need only enough blocks after matureBlock
	// to generate the tx curve
	*maxBlocks = *matureBlock + len(s.txCurve) + 1
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
		log.Printf("Missing key: '%s', delete cert: '%s' to auto-generate a new cert pair", KeyFile, CertFile)
		return ErrInvalidCertPair
	case !haveCert && haveKey:
		log.Printf("Missing cert: '%s', delete key: '%s' to auto-generate a new cert pair", CertFile, KeyFile)
		return ErrInvalidCertPair
	case !haveCert:
		// generate new cert pair if both cert and key are missing
		err := genCertPair(CertFile, KeyFile)
		if err != nil {
			return err
		}
	}

	ntfnHandlers := &rpc.NotificationHandlers{
		OnBlockConnected: func(hash *btcwire.ShaHash, height int32) {
			block := &Block{
				hash:   hash,
				height: height,
			}
			select {
			case s.com.blockQueue.enqueue <- block:
			case <-s.com.exit:
			}
		},
		OnTxAccepted: func(hash *btcwire.ShaHash, amount btcutil.Amount) {
			s.com.timeReceived <- time.Now()
		},
	}

	log.Println("Starting node on simnet...")
	args, err := NewBtcdArgs("node")
	if err != nil {
		log.Printf("Cannot create node args: %v", err)
		return err
	}
	logFile, err := getLogFile(args.prefix)
	if err != nil {
		log.Printf("Cannot get log file, logging disabled: %v", err)
	}
	node, err := NewNodeFromArgs(args, ntfnHandlers, logFile)
	if err != nil {
		log.Printf("%s: Cannot create node: %v", node, err)
		return err
	}
	if err := node.Start(); err != nil {
		log.Printf("%s: Cannot start node: %v", node, err)
		return err
	}
	if err := node.Connect(); err != nil {
		log.Printf("%s: Cannot connect to node: %v", node, err)
		return err
	}

	// Register for block notifications.
	if err := node.client.NotifyBlocks(); err != nil {
		log.Printf("%s: Cannot register for block notifications: %v", node, err)
		return err
	}

	// Register for transaction notifications
	if err := node.client.NotifyNewTransactions(false); err != nil {
		log.Printf("%s: Cannot register for transactions notifications: %v", node, err)
		return err
	}

	for i := 0; i < *numActors; i++ {
		a, err := NewActor(node, uint16(18557+i))
		if err != nil {
			log.Printf("%s: Cannot create actor: %v", a, err)
			continue
		}
		s.actors = append(s.actors, a)
	}

	// if we receive an interrupt, proceed to shutdown
	addInterruptHandler(func() {
		close(s.com.exit)
	})

	// Start simulation.
	tpsChan, tpbChan := s.com.Start(s.actors, node, s.txCurve)
	s.com.WaitForShutdown()

	tps, ok := <-tpsChan
	if ok && !math.IsNaN(tps) {
		log.Printf("Average transactions per sec: %.2f", tps)
	}

	tpb, ok := <-tpbChan
	if ok && tpb > 0 {
		log.Printf("Maximum transactions per block: %v", tpb)
	}
	return nil
}
