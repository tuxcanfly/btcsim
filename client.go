package main

import (
	"fmt"
	"net"
	"net/rpc"
)

type Listener struct {
	port   string
	height int32
}

func NewListener(port string) *Listener {
	return &Listener{
		port: port,
	}
}

func (l *Listener) listen() error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%s", l.port))
	if err != nil {
		return err
	}

	inbound, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}
	if err := rpc.Register(l); err != nil {
		return err
	}
	rpc.Accept(inbound)
	return nil
}

func (l *Listener) OnBlockConnected(hash string, ack *bool) error {
	l.height += 1
	fmt.Printf("\r%d/%d", l.height, *startBlock)
	return nil
}
