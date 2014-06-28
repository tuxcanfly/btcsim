// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	rpc "github.com/conformal/btcrpcclient"
	"github.com/conformal/btcutil"
)

var (
	shutdownChannel = make(chan bool)
)

// ChainServer describes the arguments necessary to connect a btcwallet
// instance to a btcd websocket RPC server.
type ChainServer struct {
	connect  string
	user     string
	pass     string
	certPath string
	keyPath  string
	cert     []byte
}

// For now, hardcode a single already-running btcd connection that is used for
// each actor. This should be changed to start a new btcd with the --simnet
// flag, and each actor can connect to the spawned btcd process.
var defaultChainServer = ChainServer{
	connect: "localhost:18556", // local simnet btcd
	user:    "rpcuser",
	pass:    "rpcpass",
}

// Communication is consisted of the necessary primitives used
// for communication between the main goroutine and actors.
type Communication struct {
	upstream   chan btcutil.Address
	downstream chan btcutil.Address
	stop       chan struct{}
}

const connRetry = 15

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(int64(time.Now().Nanosecond()))

	// Number of actors
	var actorsAmount = 1
	actors := make([]*Actor, 0, actorsAmount)
	com := Communication{
		upstream:   make(chan btcutil.Address, actorsAmount),
		downstream: make(chan btcutil.Address, actorsAmount),
		stop:       make(chan struct{}, actorsAmount),
	}

	btcdHomeDir := btcutil.AppDataDir("btcd", false)
	defaultChainServer.certPath = filepath.Join(btcdHomeDir, "rpc.cert")
	defaultChainServer.keyPath = filepath.Join(btcdHomeDir, "rpc.key")
	cert, err := ioutil.ReadFile(defaultChainServer.certPath)
	if err != nil {
		log.Fatalf("Cannot read certificate: %v", err)
	}
	defaultChainServer.cert = cert

	btcdArgs := []string{
		"--simnet",
		"-u" + defaultChainServer.user,
		"-P" + defaultChainServer.pass,
		"--rpccert=" + defaultChainServer.certPath,
		"--rpckey=" + defaultChainServer.keyPath,
		"--profile=",
	}

	log.Println("Starting btcd on simnet...")
	btcd := exec.Command("btcd", btcdArgs...)
	if err := btcd.Start(); err != nil {
		log.Fatalf("Couldn't start btcd: %v", err)
	}

	// Create and start RPC client.
	rpcConf := rpc.ConnConfig{
		Host:         defaultChainServer.connect,
		Endpoint:     "ws",
		User:         defaultChainServer.user,
		Pass:         defaultChainServer.pass,
		Certificates: defaultChainServer.cert,
	}

	var client *rpc.Client
	for i := 0; i < connRetry; i++ {
		if client, err = rpc.New(&rpcConf, nil); err != nil {
			time.Sleep(time.Duration(i) * 50 * time.Millisecond)
			continue
		}
		break
	}
	if client == nil {
		log.Printf("Cannot start btcd rpc client: %v", err)
		Close(actors, btcd)
		return
	}

	for i := 0; i < actorsAmount; i++ {
		a, err := NewActor(&defaultChainServer, uint16(18557+i))
		if err != nil {
			log.Printf("Cannot create actor on %s: %v", "localhost:"+a.args.port, err)
			continue
		}
		actors = append(actors, a)
	}

	addInterruptHandler(func () {
		Close(actors, btcd)
	})

	// Start actors.
	for _, a := range actors {
		a.wg.Add(1)
		go func(a *Actor, com Communication) {
			if err := a.Start(os.Stderr, os.Stdout, com); err != nil {
				log.Printf("Cannot start actor on %s: %v", "localhost:"+a.args.port, err)
				// TODO: reslice actors when one actor cannot start
			}
		}(a, com)
	}

	addressTable := make([]btcutil.Address, actorsAmount)
	for i := 0; i < actorsAmount; i++ {
		addressTable[i] = <-com.upstream
	}

	// Start mining.
	miner, err := NewMiner(addressTable, com.stop)
	if err != nil && miner == nil { // Miner didn't start at all
		Close(actors, btcd)
		return
	} else if err != nil && miner != nil { // Miner started so we have to shut it down
		miner.Shutdown()
		Close(actors, btcd)
		return
	}

	addInterruptHandler(func () {
		miner.Shutdown()
	})

	// Add mining btcd listen interface as a node
	client.AddNode("localhost:18550", rpc.ANAdd)

	go func() {
	out:
		for {
			select {
			case addr := <-com.upstream:
				com.downstream <- addr
			case <-com.stop:
				break out
			}
		}
		Close(actors, btcd)
		miner.Shutdown()
		shutdownChannel <- true
	}()

	// TODO: Collect statistics from the blockchain
	<-shutdownChannel
}

// Close sends close signal to actors and the exits initial btcd process.
func Close(actors []*Actor, btcd *exec.Cmd) {
	err := Exit(btcd)
	if err != nil {
		log.Printf("Cannot kill initial btcd process: %v", err)
	}

	for _, a := range actors {
		if err := a.Stop(); err != nil {
			log.Printf("Cannot stop actor on %s: %v", "localhost:"+a.args.port, err)
		}
		if err := a.Cleanup(); err != nil {
			log.Printf("Cannot cleanup actor on %s directory: %v", "localhost:"+a.args.port, err)
		}
		a.WaitForShutdown()
		log.Printf("Actor on %s shutdown successfully", "localhost:"+a.args.port)
	}
}
