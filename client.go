package main

import (
	"fmt"
	"net"
	"net/rpc"

	"github.com/conformal/btcrpcclient"
	"github.com/conformal/btcwire"
)

type Listener struct {
	name     string
	port     string
	height   int32
	handlers *btcrpcclient.NotificationHandlers
}

func NewListener(name, port string, handlers *btcrpcclient.NotificationHandlers) *Listener {
	return &Listener{
		name:     name,
		port:     port,
		handlers: handlers,
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
	if err := rpc.RegisterName(l.name, l); err != nil {
		return err
	}
	rpc.Accept(inbound)
	return nil
}

func (l *Listener) OnBlockConnected(hashStr string, ack *bool) error {
	l.height += 1
	hash, err := btcwire.NewShaHashFromStr(hashStr)
	if err != nil {
		return err
	}
	l.handlers.OnBlockConnected(hash, l.height)
	return nil
}

func (l *Listener) OnTxAccepted(hashStr string, ack *bool) error {
	hash, err := btcwire.NewShaHashFromStr(hashStr)
	if err != nil {
		return err
	}
	l.handlers.OnTxAccepted(hash, 0)
	return nil
}
