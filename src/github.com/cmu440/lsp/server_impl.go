// Contains the implementation of a LSP server.

package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)
import "github.com/cmu440/lspnet"

type server struct {
	params                *Params
	connections           []*my_client
	read_msg_channel      chan *addr_msg
	write_msg_channel     chan *addr_msg
	read_called_channel   chan int
	read_response_channel chan *Message
	write_data_channel    chan *Message
	close_client          chan int
	close_main            chan int
	main_closed           chan int
	request_close         bool
	udp_conn              *lspnet.UDPConn
	connected_total       int
	unread_total          int
	read_request_channel  chan int
	can_read_channel      chan int
	write_request_channel chan int
	can_write_channel     chan int
}

type addr_msg struct {
	cliAddr *lspnet.UDPAddr
	msg     *Message
}

type my_client struct {
	connId                    int
	addr                      *lspnet.UDPAddr
	unread_msgs               []*Message
	unack_msgs                int
	closed                    bool
	close_channel             chan int
	msg_sent                  int
	last_sent_seq             int
	last_received_time_passed int
	last_send_time_passed     int
	unack_msgs_list           []*unack_msg
	waiting_msgs              []*addr_msg
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	//fmt.Println("server start")
	ticker := time.NewTicker(time.Duration(params.EpochMillis) * time.Millisecond)

	addr, err := lspnet.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	conn, err := lspnet.ListenUDP("udp", addr)

	new_server := &server{
		params,
		[]*my_client{},
		make(chan *addr_msg),
		make(chan *addr_msg),
		make(chan int),
		make(chan *Message),
		make(chan *Message),
		make(chan int),
		make(chan int),
		make(chan int),
		false,
		conn,
		0,
		0,
		make(chan int),
		make(chan int),
		make(chan int),
		make(chan int),
	}

	go Read_routine(conn, new_server)
	go Write_routine(conn, new_server)
	go Main_routine(new_server, ticker)
	return new_server, err
}

func Main_routine(S *server, ticker *time.Ticker) {
	all_client_dropped := false
	can_write := true
	can_read := true
	for {
		select {
		case <-ticker.C:
			for _, c := range S.connections {
				c.last_received_time_passed += 1
				if c.last_received_time_passed > S.params.EpochLimit {
					for j, v := range S.connections {
						if c.connId == v.connId {
							S.connections = append(S.connections[:j], S.connections[(j+1):]...)
							break
						}
					}
					if len(S.connections) == 0 {
						all_client_dropped = true
						if S.unread_total == 0 {
							select {
							case S.can_read_channel <- 0:
							default:
							}
						}
					}
					continue
				}

				c.last_send_time_passed += 1
				if c.last_send_time_passed > 1 {
					heartbeat_msg := &Message{
						Type:     MsgAck,
						ConnID:   c.connId,
						SeqNum:   0,
						Size:     0,
						Checksum: 0,
						Payload:  nil,
					}
					S.write_msg_channel <- &addr_msg{c.addr, heartbeat_msg}
					c.last_send_time_passed = 0
				}
				for _, m := range c.unack_msgs_list {
					m.currentBackOff += 1
					if m.currentBackOff > m.MaxBackOff {
						retrans_msg := &addr_msg{c.addr, m.msg}
						S.write_msg_channel <- retrans_msg
						c.last_send_time_passed = 0
						m.currentBackOff = 0
						if m.MaxBackOff == 0 {
							m.MaxBackOff = 1
						} else {
							m.MaxBackOff = 2 * m.MaxBackOff
						}
					}
				}
			}
		default:
		}

		for _, v := range S.connections {
			if len(v.waiting_msgs) > 0 && (len(v.unack_msgs_list) < S.params.WindowSize) &&
				(len(v.unack_msgs_list) < S.params.MaxUnackedMessages) {
				select {
				case S.write_msg_channel <- v.waiting_msgs[0]:
					v.unack_msgs_list = append(v.unack_msgs_list, &unack_msg{
						msg:            v.waiting_msgs[0].msg,
						currentBackOff: 0,
						MaxBackOff:     0,
					})
					v.waiting_msgs = v.waiting_msgs[1:]
					v.unack_msgs += 1
					break
				default:
				}
			}
			if len(v.unread_msgs) > 0 {
				if v.unread_msgs[0].SeqNum == v.last_sent_seq+1 {
					select {
					case S.read_response_channel <- v.unread_msgs[0]:
						v.last_sent_seq = v.unread_msgs[0].SeqNum
						v.unread_msgs = v.unread_msgs[1:]
						S.unread_total -= 1
						break
					default:
					}
				}
			}

			if (all_client_dropped) && S.unread_total == 0 {
				select {
				case S.can_read_channel <- 0:
				default:
				}
			}
			if S.request_close {
				empty := true
				for _, e := range S.connections {
					if len(e.unack_msgs_list) != 0 || len(e.waiting_msgs) != 0 {
						empty = false
						break
					}
				}
				if empty && S.unread_total == 0 {
					S.main_closed <- 1
					return
				}
			}

		}
		select {
		case <-S.close_main:
			empty := true
			can_write = false
			can_read = false
			S.request_close = true
			for _, e := range S.connections {
				if len(e.unack_msgs_list) != 0 || len(e.waiting_msgs) != 0 {
					empty = false
					break
				}
			}
			if empty && S.unread_total == 0 {
				S.main_closed <- 1
				return
			}
		case id := <-S.close_client:
			for i, m := range S.connections {
				if m.connId == id {
					S.connections = append(S.connections[:i], S.connections[i+1:]...)
					break
				}
			}
		case <-S.read_request_channel:
			if S.unread_total > 0 {
				S.can_read_channel <- 1
				continue
			}
			if all_client_dropped {
				S.can_read_channel <- 0
				continue
			}
			if can_read {
				S.can_read_channel <- 1
				continue
			}
			S.can_read_channel <- 0
		case <-S.write_request_channel:
			if can_write {
				S.can_write_channel <- 1
			} else {
				S.can_write_channel <- 0
			}

		case new_msg := <-S.read_msg_channel:
			if new_msg.msg.Type == MsgConnect {
				new_response := &Message{MsgAck, S.connected_total, 0, 0, 0, nil}
				new_addr_msg := &addr_msg{new_msg.cliAddr, new_response}
				S.write_msg_channel <- new_addr_msg
				new_client := &my_client{S.connected_total, new_msg.cliAddr, nil, 0,
					false, make(chan int), 0, 0, 0, 0, nil, nil}
				S.connections = append(S.connections, new_client)
				S.connected_total += 1
				continue
			} else if new_msg.msg.Type == MsgData {
				new_response := &Message{MsgAck, new_msg.msg.ConnID, new_msg.msg.SeqNum, 0, 0, nil}
				new_addr_msg := &addr_msg{new_msg.cliAddr, new_response}
				S.write_msg_channel <- new_addr_msg
				id := new_msg.msg.ConnID
				//add to unread messages list
				for _, v := range S.connections {
					if v.connId == id {
						v.last_received_time_passed = 0
						v.last_send_time_passed = 0
						if v.last_sent_seq >= new_msg.msg.SeqNum {
							break
						}
						cur_list := v.unread_msgs
						flag := false
						for i, m := range cur_list {
							if m.SeqNum == new_msg.msg.SeqNum {
								flag = true
								break
							}
							if m.SeqNum > new_msg.msg.SeqNum {
								temp1 := make([]*Message, len(v.unread_msgs[i:]))
								copy(temp1, v.unread_msgs[i:])
								v.unread_msgs = append(append(v.unread_msgs[:i], new_msg.msg), temp1...)
								S.unread_total += 1
								flag = true
								break
							}
						}
						if flag == false {
							v.unread_msgs = append(v.unread_msgs, new_msg.msg)
							S.unread_total += 1
						}
						break
					}
				}
			} else if new_msg.msg.Type == MsgAck {

				for _, v := range S.connections {
					if v.connId == new_msg.msg.ConnID {
						for i, m := range v.unack_msgs_list {
							if m.msg.SeqNum == new_msg.msg.SeqNum {
								v.unack_msgs_list = append(v.unack_msgs_list[:i], v.unack_msgs_list[(i+1):]...)
								v.unack_msgs -= 1
								break
							}
						}
						v.last_received_time_passed = 0
						break
					}
				}
			}
		case new_data_write := <-S.write_data_channel:
			id := new_data_write.ConnID
			var matched_client *my_client
			for _, v := range S.connections {
				if v.connId == id {
					matched_client = v
					break
				}
			}
			matched_client.msg_sent += 1
			new_data_write.SeqNum = matched_client.msg_sent
			new_addr_msg := &addr_msg{matched_client.addr, new_data_write}

			if len(matched_client.unack_msgs_list) < S.params.WindowSize && (len(matched_client.unack_msgs_list) < S.params.MaxUnackedMessages) {
				S.write_msg_channel <- new_addr_msg
				matched_client.last_send_time_passed = 0
				matched_client.unack_msgs_list = append(matched_client.unack_msgs_list, &unack_msg{
					msg:            new_data_write,
					currentBackOff: 0,
					MaxBackOff:     0,
				})
				matched_client.unack_msgs += 1
			} else {
				matched_client.waiting_msgs = append(matched_client.waiting_msgs, new_addr_msg)
			}
		default:
			continue
		}
	}

}
func Write_routine(conn *lspnet.UDPConn, S *server) {
	for {
		select {
		case new_write := <-S.write_msg_channel:
			new_msg := new_write.msg
			buffer, err1 := json.Marshal(new_msg)
			if err1 != nil {
				return
			}
			_, err2 := conn.WriteToUDP(buffer, new_write.cliAddr)
			if err2 != nil {
				return
			}
		}
	}
}

func Read_routine(conn *lspnet.UDPConn, S *server) {
	for {
		buffer := make([]byte, 2000)
		n, cliAddr, err1 := conn.ReadFromUDP(buffer)
		if err1 != nil {
			return
		}
		var new_msg Message
		err2 := json.Unmarshal(buffer[:n], &new_msg)
		if err2 != nil {
			return
		}
		if len(new_msg.Payload) < new_msg.Size {
			return
		}
		if len(new_msg.Payload) > new_msg.Size {
			new_msg.Payload = new_msg.Payload[:new_msg.Size]
		}
		new_addr_msg := &addr_msg{
			cliAddr: cliAddr,
			msg:     &new_msg,
		}
		S.read_msg_channel <- new_addr_msg
	}
}

//
func (s *server) Read() (int, []byte, error) {
	s.read_request_channel <- 1
	c := <-s.can_read_channel
	if c == 0 {
		return 0, nil, errors.New("connection closed")
	}
	select {
	case new_msg := <-s.read_response_channel:
		return new_msg.ConnID, new_msg.Payload, nil
	case <-s.can_read_channel:
		return 0, nil, errors.New("connection closed")
	}
}

func (s *server) Write(connId int, payload []byte) error {
	s.write_request_channel <- 1
	c := <-s.can_write_channel
	if c == 0 {
		return errors.New("connection closed")
	}
	new_msg := &Message{
		Type:     MsgData,
		ConnID:   connId,
		SeqNum:   0,
		Size:     len(payload),
		Checksum: 0,
		Payload:  payload,
	}
	s.write_data_channel <- new_msg
	return nil
}

func (s *server) CloseConn(connId int) error {
	s.close_client <- connId
	return nil
}

func (s *server) Close() error {
	s.close_main <- 1
	<-s.main_closed
	s.udp_conn.Close()
	return nil
}
