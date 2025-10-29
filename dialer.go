package main

import (
	"crypto/tls"
	"net"
)

// TLSDialer dials a TLS connection for gosmpp connector.
// NOTE: InsecureSkipVerify is true here to match the original example.
// For production, validate server certificates properly.
var TLSDialer = func(addr string) (net.Conn, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}
	return tls.Dial("tcp", addr, conf)
}
