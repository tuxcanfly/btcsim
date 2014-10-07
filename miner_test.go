package main

import (
	"os"
	"testing"

	"github.com/conformal/btcutil"
)

func TestNewMiner(t *testing.T) {
	// generate new cert pair if both cert and key are missing
	err := genCertPair(CertFile, KeyFile)
	if err != nil {
		t.Errorf("NewBtcdArgs error: %v", err)
	}
	defer os.Remove(CertFile)
	defer os.Remove(KeyFile)
	fakeArgs, err := NewBtcdArgs("node")
	if err != nil {
		t.Errorf("NewBtcdArgs error: %v", err)
	}
	fakeNode, err := NewNodeFromArgs(fakeArgs, nil, nil)
	if err != nil {
		t.Errorf("NewNodeFromArgs error: %v", err)
	}
	if err := fakeNode.Start(); err != nil {
		t.Errorf("fakeNode.Start error: %v", err)
	}
	defer fakeNode.Shutdown()
	actor, err := NewActor(fakeNode, 18557)
	if err != nil {
		t.Errorf("NewActor error: %v", err)
	}
	defer actor.Shutdown()
	fakeCom := NewCommunication()
	go func() {
		for err := range fakeCom.errChan {
			t.Errorf("actor.Start error: %v", err)
		}
	}()
	addrs := make(chan btcutil.Address)
	go func() {
		for addr := range actor.miningAddr {
			addrs <- addr
		}
	}()
	if err := actor.Start(nil, nil, fakeCom); err != nil {
		t.Errorf("actor.Start error: %v", err)
	}
	miningaddrs := make([]btcutil.Address, 1)
	for addr := range addrs {
		miningaddrs = append(miningaddrs, addr)
	}
	exit := make(chan struct{})
	height := make(chan int32)
	txpool := make(chan struct{})
	miner, err := NewMiner(miningaddrs, exit, height, txpool)
	if err != nil {
		t.Errorf("NewMiner error: %v", err)
	}
	defer miner.Shutdown()
}
