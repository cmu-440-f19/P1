package lsp

import (
	"github.com/cmu440/lspnet"
)

type parsedMsg struct {
	Addr lspnet.UDPAddr
	ConnID   int
	SeqNum   int
	Type MsgType
	Payload []byte
}

func Listen(conn lspnet.UDPConn, msgChan chan parsedMsg) error {
	return nil
}

func Write(addr lspnet.UDPAddr, connID int, dataChan chan []byte, ackChan chan int, closeChan chan int) error {
	return nil
}
