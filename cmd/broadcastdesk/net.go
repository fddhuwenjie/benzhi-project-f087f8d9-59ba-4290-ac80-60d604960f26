package main

import (
	"io"
	"net"
)

func netListen(addr string) (net.Listener, error) { return net.Listen("tcp", addr) }
func ioReadAll(r io.Reader) ([]byte, error)       { return io.ReadAll(r) }
