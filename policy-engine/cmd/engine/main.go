package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"policy-engine/extproc"
)

func main() {
	port := flag.Int("port", 9001, "gRPC port")
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen to %d: %v", *port, err)
	}

	go func() {
		gs := extproc.NewServer()
		log.Printf("starting gRPC server on: %d\n", *port)
		gs.Serve(lis)
	}()
	select {}
}
