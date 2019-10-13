// Contains the implementation of a LSP client.

package lsp

import (
	"encoding/json"
	"errors"
	"github.com/cmu440/lspnet"
	"time"
)

type client struct {
	params                   *Params
	connID                   int
	conn                     *lspnet.UDPConn
	server_addr              *lspnet.UDPAddr
	unread_msgs              []*Message
	unack_msg                int
	read_msg_channel         chan *Message
	write_msg_channel        chan *Message
	read_called              bool
	read_response_channel    chan *Message
	write_data_channel       chan *Message
	last_send_time_passed    int
	last_receive_time_passes int
	connection_received      chan bool
	unack_msgs_list          []*unack_msg
	last_read_seq            int
	waiting_msgs             []*Message
	close_main_channel       chan int
	main_closed_channel      chan int
	can_read_channel         chan int
	write_request_channel    chan int
	can_write_channel        chan int
	connection_lost          bool
	sent_msg                 []*Message
	total_unread_msgs        int
}

type unack_msg struct {
	msg            *Message
	currentBackOff int
	MaxBackOff     int
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	addr, _ := lspnet.ResolveUDPAddr("udp", hostport)
	conn, _ := lspnet.DialUDP("udp", nil, addr)

	new_client := &client{
		params:                   params,
		connID:                   0,
		conn:                     conn,
		server_addr:              addr,
		unread_msgs:              nil,
		unack_msg:                0,
		read_msg_channel:         make(chan *Message),
		write_msg_channel:        make(chan *Message),
		read_called:              false,
		read_response_channel:    make(chan *Message),
		write_data_channel:       make(chan *Message),
		last_send_time_passed:    0,
		last_receive_time_passes: 0,
		connection_received:      make(chan bool),
		unack_msgs_list:          nil,
		last_read_seq:            0,
		waiting_msgs:             nil,
		close_main_channel:       make(chan int),
		main_closed_channel:      make(chan int),
		can_read_channel:         make(chan int),
		write_request_channel:    make(chan int),
		can_write_channel:        make(chan int),
		connection_lost:          false,
		sent_msg:                  nil,
		total_unread_msgs:   0,
	}

	ticker := time.NewTicker(time.Duration(params.EpochMillis) * time.Millisecond)

	go connect_client_routine(new_client, ticker)
	conenction_request := &Message{MsgConnect, 0, 0, 0, 0, nil}
	write_buffer, _ := json.Marshal(conenction_request)
	_, err2 := new_client.conn.Write(write_buffer)

	for {
		var buffer [2000]byte
		n, err2 := new_client.conn.Read(buffer[0:])
		if err2 != nil {
			continue
		}
		var new_msg Message
		err3 := json.Unmarshal(buffer[:n], &new_msg)
		if err3 != nil {
			continue
		}
		new_client.connID = new_msg.ConnID
		new_client.connection_received <- true
		break
	}

	go Client_Read_routine(new_client)
	go Client_Write_routine(new_client)
	go Client_Main_routine(new_client, ticker)

	return new_client, err2
}

func connect_client_routine(new_client *client, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			conenction_request := &Message{MsgConnect, 0, 0, 0, 0, nil}
			write_buffer, _ := json.Marshal(conenction_request)
			_, err2 := new_client.conn.Write(write_buffer)
			if err2 != nil {
				continue
			}
		case <-new_client.connection_received:
			return

		}
	}

}
func Client_Main_routine(c *client, ticker *time.Ticker) {
	msg_sent := 0
	sent_msg := []*Message{}
	can_write := true
	for {
		if len(c.waiting_msgs) > 0 && (len(c.unack_msgs_list) < c.params.WindowSize) && (len(c.unack_msgs_list) < c.params.MaxUnackedMessages) {
			select {
			case c.write_msg_channel <- c.waiting_msgs[0]:
				c.last_send_time_passed = 0
				c.unack_msgs_list = append(c.unack_msgs_list, &unack_msg{
					msg:            c.waiting_msgs[0],
					currentBackOff: 0,
					MaxBackOff:     0,
				})
				c.unack_msg += 1
				c.waiting_msgs = c.waiting_msgs[1:]
			default:
			}
		}

		if (len(c.unread_msgs) > 0) && (c.unread_msgs[0].SeqNum == c.last_read_seq+1) {
			select {
			case c.read_response_channel <- c.unread_msgs[0]:
				sent_msg = append(sent_msg, c.unread_msgs[0])
				c.unread_msgs = c.unread_msgs[1:]
				c.total_unread_msgs -= 1
				c.last_read_seq = c.last_read_seq + 1
			default:
			}
		}
		if c.total_unread_msgs == 0 && c.connection_lost {
			select {
			case c.can_read_channel <- 0:
			default:
			}

		}
		if len(c.unack_msgs_list) == 0 && len(c.waiting_msgs) == 0 && c.total_unread_msgs == 0 && can_write == false {
			c.main_closed_channel <- 1
			return
		}

		select {
		case <-ticker.C:
			TrackTicker(c)

		case <-c.close_main_channel:
			can_write = false
			if len(c.unack_msgs_list) == 0 && len(c.waiting_msgs) == 0 && c.total_unread_msgs == 0 {
				select {
				case c.can_read_channel <- 0:
				default:
				}
				c.main_closed_channel <- 1
				return
			}
		case <-c.write_request_channel:
			if can_write {
				c.can_write_channel <- 1
			} else {
				c.can_write_channel <- 0
			}
		case new_msg := <-c.read_msg_channel:
			HandleReadMsgs(new_msg, c)

		case new_write := <-c.write_data_channel:
			msg_sent++
			new_write.SeqNum = msg_sent
			if (c.unack_msg < c.params.WindowSize) && (len(c.unack_msgs_list) < c.params.MaxUnackedMessages) {
				c.write_msg_channel <- new_write
				c.last_send_time_passed = 0
				c.unack_msgs_list = append(c.unack_msgs_list, &unack_msg{
					msg:            new_write,
					currentBackOff: 0,
					MaxBackOff:     0,
				})
				c.unack_msg += 1
			} else {
				c.waiting_msgs = append(c.waiting_msgs, new_write)
			}
		default:
			time.Sleep(10 * time.Microsecond)
		}
	}
}

func Client_Read_routine(c *client) {
	for {
		var buffer [2000]byte
		n, err1 := c.conn.Read(buffer[0:])
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
		c.read_msg_channel <- &new_msg

	}
}

func Client_Write_routine(c *client) {
	for {
		select {
		case new_write := <-c.write_msg_channel:

			buffer, err1 := json.Marshal(new_write)
			if err1 != nil {
				return
			}
			_, err2 := c.conn.Write(buffer)
			if err2 != nil {
				return
			}
		}
	}
}

func (c *client) ConnID() int {
	return c.connID
}

func (c *client) Read() ([]byte, error) {
	select {
	case new_msg := <-c.read_response_channel:
		return new_msg.Payload, nil
	case <-c.can_read_channel:
		return nil, errors.New("connection closed")
	}
}

func (c *client) Write(payload []byte) error {
	c.write_request_channel <- 1
	r := <-c.can_write_channel
	if r == 0 {
		return errors.New("connection closed")
	}
	new_msg := &Message{
		Type:     MsgData,
		ConnID:   c.connID,
		SeqNum:   0,
		Size:     len(payload),
		Checksum: 0,
		Payload:  payload,
	}
	c.write_data_channel <- new_msg
	return nil
}

func (c *client) Close() error {
	c.close_main_channel <- 1
	<-c.main_closed_channel
	c.conn.Close()
	return nil
}

func TrackTicker(c *client){
	c.last_receive_time_passes += 1
	if (c.last_receive_time_passes > c.params.EpochLimit) && (len(c.unread_msgs) == 0) {
		c.connection_lost = true
	}

	c.last_send_time_passed += 1
	if c.last_send_time_passed > 1 {
		heartbeat_msg := &Message{
			Type:     MsgAck,
			ConnID:   c.connID,
			SeqNum:   0,
			Size:     0,
			Checksum: 0,
			Payload:  nil,
		}
		c.write_msg_channel <- heartbeat_msg
		c.last_send_time_passed = 0
	}

	for _, m := range c.unack_msgs_list {
		m.currentBackOff += 1
		if m.currentBackOff > m.MaxBackOff {
			c.write_msg_channel <- m.msg
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

func HandleReadMsgs(new_msg *Message, c *client){
	c.last_receive_time_passes = 0
	if new_msg.Type == MsgData {
		new_response := &Message{MsgAck, c.connID, new_msg.SeqNum, 0, 0, nil}
		c.write_msg_channel <- new_response
		flag := false
		for _, m := range c.sent_msg {
			if m.SeqNum == new_msg.SeqNum {
				flag = true
			}
		}
		if flag {
			return
		}
		for i, m := range c.unread_msgs {
			if m.SeqNum == new_msg.SeqNum {
				flag = true
				break
			}
			if m.SeqNum > new_msg.SeqNum {
				temp1 := make([]*Message, len(c.unread_msgs[i:]))
				copy(temp1, c.unread_msgs[i:])
				c.unread_msgs = append(append(c.unread_msgs[:i], new_msg), temp1...)
				c.total_unread_msgs += 1
				flag = true
				break
			}
		}
		if flag == false {
			c.unread_msgs = append(c.unread_msgs, new_msg)
			c.total_unread_msgs += 1
		}

		if (len(c.unread_msgs) > 0) && (c.unread_msgs[0].SeqNum == c.last_read_seq+1) && (len(c.unack_msgs_list) < c.params.MaxUnackedMessages) {
			select {
			case c.read_response_channel <- c.unread_msgs[0]:
				c.sent_msg = append(c.sent_msg, c.unread_msgs[0])
				c.unread_msgs = c.unread_msgs[1:]
				c.last_read_seq = c.last_read_seq + 1
				c.total_unread_msgs -= 1
				break
			default:
			}
		}

	} else if new_msg.Type == MsgAck {
		if new_msg.SeqNum == 0 {
			return
		}
		for i, m := range c.unack_msgs_list {
			if m.msg.SeqNum == new_msg.SeqNum {
				c.unack_msgs_list = append(c.unack_msgs_list[:i], c.unack_msgs_list[(i+1):]...)
				c.unack_msg -= 1
				break
			}
		}

	}
}