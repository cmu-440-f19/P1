package lsp

import (
	"github.com/cmu440/lspnet"
)

type MsgWrapper struct {
	addr lspnet.UDPAddr
	msg Message
}

func Listen(ntwk string, laddr *lspnet.UDPAddr, msgChan chan MsgWrapper) error {
	return nil
}

func Write(addr lspnet.UDPAddr, connID int, dataChan chan []byte, ackChan chan int, closeChan chan int) error {
	return nil
}
