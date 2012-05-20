package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type stream struct {
	client *client
}

func newStream(client *client) *stream {
	return &stream{client: client}
}

type sockStream struct {
	net.Conn
	BytesRead    uint32
	BytesWritten uint32
}

func newSockStream(c net.Conn) *sockStream {
	return &sockStream{Conn: c}
}

func (ss *sockStream) Read(b []byte) (n int, err error) {
	n, err = ss.Conn.Read(b)
	ss.BytesRead += uint32(n)
	return
}

func (ss *sockStream) Write(b []byte) (n int, err error) {
	n, err = ss.Conn.Write(b)
	ss.BytesWritten += uint32(n)
	return
}

type client struct {
	mutex             sync.Mutex
	c                 *sockStream
	server            *Server
	incompletePackets map[uint16]Payload // ChunkId: Header
	lastReadHeaders   map[uint16]*Header // ChunkId: Header
	lastWriteHeaders  map[uint32]*Header // StreamId: Header
	readSize          uint32
	chunkSize         uint32 // overflow?
	winSize           uint32
	time              int64
	chunkWriter       chan chunkAndReply
	nextStreamId      uint32
	nextChunkId       uint16
}

type chunkAndReply struct {
	chunk chunk
	reply chan error
}

func newClient(c net.Conn, server *Server) *client {
	c := &client{c: newSockStream(c), server: server,
		incompletePackets: make(map[uint16]Payload),
		lastReadHeaders:   make(map[uint16]*Header),
		lastWriteHeaders:  make(map[uint32]*Header),
		chunkSize:         128,
		time:              time.Now().Unix(),
		chunkWriter:       make(chan chunkAndReply),
		nextStreamId:      1,
		nextChunkId:       3,
	}
	// c.lastWriteHeaders[message.StreamId] = new(Header)
	// c.lastWriteHeaders[message.StreamId].chunkId = 2
	return c
}

func (c *client) relativeTime() uint32 {
	return uint32(time.Now().Unix() - c.time)
}

func (c *client) serve() {
	err := c.handShake()
	if err != nil {
		log.Printf("handshake error: %s", err)
		return
	}
	go c.writeLoop()
	err = c.readLoop()
	if err != nil {
		log.Printf("read error: %s", err)
		return
	}
}

func (c *client) handShake() error {
	S0 := []byte{0x03}
	C0 := make([]byte, 1)
	S1 := make([]byte, 1536)
	C1 := make([]byte, 1536)
	_, err := io.ReadFull(c.c, C0)
	if err != nil {
		return err
	}
	_, err = c.c.Write(S0)
	if err != nil {
		return err
	}
	_, err = c.c.Write(S1)
	if err != nil {
		return err
	}
	_, err = io.ReadFull(c.c, C1)
	if err != nil {
		return err
	}
	_, err = c.c.Write(C1)
	if err != nil {
		return err
	}
	_, err = io.ReadFull(c.c, S1)
	if err != nil {
		return err
	}
	return nil
}

func (c *client) readLoop() error {
	for {
		u8 := make([]byte, 1)
		u16 := make([]byte, 2)
		u32 := make([]byte, 4)
		_, err := io.ReadFull(c.c, u8)
		if err != nil {
			return err
		}
		chunktype := (u8[0] & 0xC0) >> 6
		chunkid := uint16(u8[0] & 0x3F)
		switch chunkid {
		case 0:
			_, err := io.ReadFull(c.c, u8)
			if err != nil {
				return err
			}
			chunkid = 64 + uint16(u8[0])
		case 1:
			_, err := io.ReadFull(c.c, u16)
			if err != nil {
				return err
			}
			chunkid = 64 + uint16(u16[0]) + 256*uint16(u16[1])
		}
		if c.lastReadHeaders[chunkid] == nil {
			c.lastReadHeaders[chunkid] = new(Header)
			c.lastReadHeaders[chunkid].chunkId = chunkid
		}
		if chunktype != 3 && c.incompletePackets[chunkid] != nil {
			return errors.New("incomplete message droped")
		}
		switch chunktype {
		case 0:
			_, err := io.ReadFull(c.c, u32[1:])
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			_, err = io.ReadFull(c.c, u32[1:])
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Length = binary.BigEndian.Uint32(u32)
			_, err = io.ReadFull(c.c, u8)
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Type = u8[0]
			_, err = io.ReadFull(c.c, u32)
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].StreamId = binary.LittleEndian.Uint32(u32)
			if c.lastReadHeaders[chunkid].Time == 0xFFFFFF {
				_, err = io.ReadFull(c.c, u32)
				if err != nil {
					return err
				}
				c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			}
			c.lastReadHeaders[chunkid].CurrentTime = c.lastReadHeaders[chunkid].Time
		case 1:
			_, err := io.ReadFull(c.c, u32[1:])
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			_, err = io.ReadFull(c.c, u32[1:])
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Length = binary.BigEndian.Uint32(u32)
			_, err = io.ReadFull(c.c, u8)
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Type = u8[0]
			if c.lastReadHeaders[chunkid].Time == 0xFFFFFF {
				_, err = io.ReadFull(c.c, u32)
				if err != nil {
					return err
				}
				c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			}
			c.lastReadHeaders[chunkid].CurrentTime += c.lastReadHeaders[chunkid].Time
		case 2:
			_, err := io.ReadFull(c.c, u32[1:])
			if err != nil {
				return err
			}
			c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			if c.lastReadHeaders[chunkid].Time == 0xFFFFFF {
				_, err = io.ReadFull(c.c, u32)
				if err != nil {
					return err
				}
				c.lastReadHeaders[chunkid].Time = binary.BigEndian.Uint32(u32)
			}
			c.lastReadHeaders[chunkid].CurrentTime += c.lastReadHeaders[chunkid].Time
		case 3:
			if c.incompletePackets[chunkid] == nil {
				c.lastReadHeaders[chunkid].CurrentTime += c.lastReadHeaders[chunkid].Time
			}
		}
		if c.incompletePackets[chunkid] == nil {
			c.incompletePackets[chunkid] = NewPayload(c.lastReadHeaders[chunkid].Length)
		}
		nRead := uint32(len(c.incompletePackets[chunkid]))
		size := c.lastReadHeaders[chunkid].Length - nRead
		if size > c.chunkSize {
			size = c.chunkSize
		}
		buf := c.incompletePackets[chunkid][nRead:nRead+size]
		_, err = io.ReadFull(c.c, buf)
		if err != nil {
			return err
		}
		c.incompletePackets[chunkid] = c.incompletePackets[chunkid][0 : nRead+size]
		if uint32(len(c.incompletePackets[chunkid])) == c.lastReadHeaders[chunkid].Length {
			message := new(Message)
			message.Header = *c.lastReadHeaders[chunkid]
			message.Payload = c.incompletePackets[chunkid]
			c.incompletePackets = nil
			if chunkid == 0x02 {
				err := c.handleProtocol(message)
				if err != nil {
					return err
				}
			} else {
				go c.handleMessage(message)
			}
		}
	}
	return nil
}

func (c *client) handleProtocol(message *Message) error {
	switch message.Type {
	case 1: // Set Chunk Size
		c.chunkSize = binary.BigEndian.Uint32(message.Payload)
	case 4:
		// deal setBufferLength
	case 5: // Window Acknowledgement Size
		c.winSize = binary.BigEndian.Uint32(message.Payload)
		c.readSize = c.c.BytesRead
	}
	return nil
}

func (c *client) handleMessage(message *Message) {
	if message.Type == 0x14 && message.StreamId == 0 {
		cmd, err := DecodeCommand0(message.Payload)
		if err != nil {
			log.Println(err)
			return
		}
		switch cmd.Name {
		case "connect":
			reply := make(chan error)
			c.server.connect <- connectAndReply{c, cmd, reply}
			err := <-reply
			log.Println(err)
		}
	}
}

func (c *client) writeMessage(message Message) (err error) {
	message.CurrentTime = c.relativeTime()
	var lastWriteHeader *Header
	var chunk chunk
	c.mutex.Lock()
	var ok bool
	if lastWriteHeader, ok = c.lastWriteHeaders[message.StreamId]; !ok {
		c.lastWriteHeaders[message.StreamId] = new(Header)
		lastWriteHeader = c.lastWriteHeaders[message.StreamId]
		lastWriteHeader.chunkId = c.nextChunkId
		c.nextChunkId++
		chunk.Type = 0
	} else {
		if lastWriteHeader.Type != message.Type || lastWriteHeader.Length != message.Length {
			chunk.Type = 1
		} else {
			message.Time = message.CurrentTime - lastWriteHeader.Time
			if message.Time != lastWriteHeader.Time {
				chunk.Type = 2
			} else {
				chunk.Type = 3
			}
		}
	}
	c.mutex.Unlock()
	lastWriteHeader.CurrentTime = message.CurrentTime
	lastWriteHeader.Length = message.Length
	lastWriteHeader.Time = message.Time
	lastWriteHeader.Type = message.Type
	lastWriteHeader.StreamId = message.StreamId
	chunk.Header = *lastWriteHeader
	payload := message.Payload
	nWrite := uint32(0)
	replyChan := make(chan error)
	for lastWriteHeader.Length-nWrite > c.chunkSize {
		chunk.Payload = payload[nWrite:c.chunkSize]
		c.chunkWriter <- chunkAndReply{chunk, replyChan}
		err = <-replyChan
		if err != nil {
			return err
		}
		nWrite += c.chunkSize
		chunk.Type = 3
	}
	chunk.Payload = payload[nWrite:]
	c.chunkWriter <- chunkAndReply{chunk, replyChan}
	err = <-replyChan
	if err != nil {
		return err
	}
	return nil
}

func (c *client) writeLoop() {
	for cnr := range c.chunkWriter {
		chunk := cnr.chunk
		reply := cnr.reply
		buf := new(bytes.Buffer)
		if chunk.chunkId < 64 {
			buf.WriteByte(byte(chunk.chunkId) | chunk.Type<<6)
		} else if chunk.chunkId < 64+256 {
			buf.WriteByte(0x00 | chunk.Type<<6)
			buf.WriteByte(byte(chunk.chunkId - 64))
		} else if chunk.chunkId < 65535 { // 64 + 256 + 256*256 out of uint16
			buf.WriteByte(0x01 | chunk.Type<<6)
			buf.WriteByte(byte((chunk.chunkId - 64) % 256))
			buf.WriteByte(byte((chunk.chunkId - 64) / 256))
		} else {
			reply <- errors.New("chunkid out of range")
			continue
		}
		switch chunk.Type {
		case 0:
			u32 := make([]byte, 4)
			if chunk.Time > 0x00FFFFFF {
				buf.Write([]byte{0x00, 0xff, 0xff, 0xff})
			} else {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32[1:])
			}
			binary.BigEndian.PutUint32(u32, chunk.Length)
			buf.Write(u32[1:])
			buf.WriteByte(chunk.Header.Type)
			binary.LittleEndian.PutUint32(u32, chunk.StreamId)
			buf.Write(u32)
			buf.Write(chunk.Payload)
			if chunk.Time > 0x00FFFFFF {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32)
			}
		case 1:
			u32 := make([]byte, 4)
			if chunk.Time > 0x00FFFFFF {
				buf.Write([]byte{0xff, 0xff, 0xff})
			} else {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32[1:])
			}
			binary.BigEndian.PutUint32(u32, chunk.Length)
			buf.Write(u32[1:])
			buf.WriteByte(chunk.Header.Type)
			buf.Write(chunk.Payload)
			if chunk.Time > 0x00FFFFFF {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32)
			}
		case 2:
			u32 := make([]byte, 4)
			if chunk.Time > 0x00FFFFFF {
				buf.Write([]byte{0x00, 0xff, 0xff, 0xff})
			} else {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32[1:])
			}
			buf.Write(chunk.Payload)
			if chunk.Time > 0x00FFFFFF {
				binary.BigEndian.PutUint32(u32, chunk.Time)
				buf.Write(u32)
			}
		case 3:
			buf.Write(chunk.Payload)
		}
		_, err := c.c.Write(buf.Bytes())
		if err != nil {
			reply <- err
		}
		reply <- nil
	}
}
