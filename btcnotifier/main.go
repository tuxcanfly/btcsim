package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"os"
)

var (
	// port
	port = flag.String("port", "19550", "RPC server port")
)

func main() {
	flag.Parse()

	fmt.Printf("%v\n", os.Args)

	client, err := rpc.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", *port))
	if err != nil {
		log.Fatal(err)
	}

	var reply bool
	err = client.Call("Listener.OnBlockConnected", os.Args[2], &reply)
	if err != nil {
		log.Fatal(err)
	}
}
