package connection_pool

import (
	"context"
	"github.com/ihciah/rabbit-tcp/block"
	"github.com/ihciah/rabbit-tcp/connection"
	"github.com/ihciah/rabbit-tcp/tunnel_pool"
	"net"
)

const (
	SendQueueSize = 24
)

type ConnectionPool struct {
	connectionMapping map[uint32]connection.Connection
	manager           Manager
	tunnelPool        *tunnel_pool.TunnelPool
	sendQueue         chan block.Block

	ctx    context.Context
	cancel context.CancelFunc
}

func NewConnectionPool(manager Manager, pool *tunnel_pool.TunnelPool) ConnectionPool {
	ctx, cancel := context.WithCancel(context.Background())
	cp := ConnectionPool{
		connectionMapping: make(map[uint32]connection.Connection),
		manager:           manager,
		tunnelPool:        pool,
		sendQueue:         make(chan block.Block, SendQueueSize),
		ctx:               ctx,
		cancel:            cancel,
	}
	go cp.SendRelay()
	go cp.RecvRelay()
	return cp
}

func (cp *ConnectionPool) NewInboundConnection() connection.Connection {
	c := connection.NewInboundConnection(cp.sendQueue)
	cp.AddConnection(c)
	return c
}

func (cp *ConnectionPool) NewOutboundConnection(connectionID uint32, address string) (connection.Connection, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	c := connection.NewOutboundConnectionWithID(conn, connectionID, cp.sendQueue)
	return c, nil
}

func (cp *ConnectionPool) AddConnection(conn connection.Connection) {
	// TODO: thread safe
	cp.connectionMapping[conn.GetConnectionID()] = conn
	go conn.Daemon(conn)
}

func (cp *ConnectionPool) RemoveConnection(conn connection.Connection) {
	// TODO: thread safe
	if _, ok := cp.connectionMapping[conn.GetConnectionID()]; ok {
		delete(cp.connectionMapping, conn.GetConnectionID())
		conn.CancelDaemon()
	}
}

// Deliver blocks from tunnelPool channel to specified connections
func (cp *ConnectionPool) RecvRelay() {
	for {
		select {
		case blk := <-cp.tunnelPool.GetRecvQueue():
			connID := blk.ConnectionID
			if conn, ok := cp.connectionMapping[connID]; ok {
				conn.GetRecvQueue() <- blk
			}
		case <-cp.ctx.Done():
			return
		}

	}
}

// Deliver blocks from connPool's sendQueue to tunnelPool
// TODO: Maybe QOS can be implemented here
func (cp *ConnectionPool) SendRelay() {
	for {
		select {
		case blk := <-cp.sendQueue:
			cp.tunnelPool.GetSendQueue() <- blk
		case <-cp.ctx.Done():
			return
		}
	}
}
