package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"os"
)

var (
	// map notification events to RPC call
	notifications = map[string]string{
		"block": "BlockNotify",
		"tx":    "WalletNotify",
	}

	// name
	name = flag.String("name", "", "RPC Listener name")

	// event
	event = flag.String("event", "block", "Notification event")

	// port
	port = flag.String("port", "19550", "RPC server port")
)

func init() {
	flag.Parse()
	if _, ok := notifications[*event]; !ok {
		log.Fatal("invalid event - must be one of 'block', 'tx'")
	}
}

func main() {
	client, err := rpc.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", *port))
	if err != nil {
		log.Fatal(err)
	}

	var reply bool
	method := fmt.Sprintf("%s.%s", *name, notifications[*event])
	arg := os.Args[4]
	err = client.Call(method, arg, &reply)
	if err != nil {
		log.Fatal(err)
	}
}
