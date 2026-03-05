package proxy

import (
	"github.com/gorilla/websocket"
)

type ClientConn struct {
	host string
	port int
	conn websocket.Conn
}

type TargetConn struct{
	host string
	port int
	conn websocket.Conn
}

type ProxyConn struct {
	client ClientConn
	target TargetConn
}
