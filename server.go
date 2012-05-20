package rtmp

import (
	"net"
	"sync"
	"encoding/binary"
	"github.com/hongruiqi/amf.go/amf0"
)

type FlashServer struct {
	mutex   sync.Mutex
	apps    map[string]Application
	clients map[string][]client
	connect chan connectAndReply
}

func NewFlashServer() *FlashServer {
	return &FlashServer{apps: make(map[string]Application), clients: make(map[string][]client), connect: make(chan connectAndReply)}
}

func (fs *FlashServer) Handle(path string, app Application) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	fs.apps[path] = app
}

type connectAndReply struct {
	c   *client
	cmd *Command
	reply chan error
}

func (fs *FlashServer) connectLoop() {
	for cnr := range s.connect {
		c := cnr.c
		cmd := cnr.cmd
		reply := cnr.reply
		// ack
		var message Message
		message.Type = WindowAckSize // const
		message.Payload = make([]byte, 4)
		binary.BigEndian.PutUint32(message.Payload, WindowSize)
		c.writeMessage(message)
		// _result or _error
		message.Type = AMF0Command
		appnameif := cmd.Object.(*amf0.ObjectType).Get("app")
		appname := ""
		if appnameif != nil {
			appname = string(appnameif.(amf0.StringType))
		}
		if app := s.fs.apps[appname]; app != nil {
			result := new(Command)
			result.Name = "_result"
			result.Tid = cmd.Tid
			ptr := amf0.NewObjectType()
			obj := *ptr
			obj[amf0.StringType("fmsVer")] = amf0.StringType("FMS/3,5,1,525")
			obj[amf0.StringType("capabilities")] = amf0.NumberType(31)
			obj[amf0.StringType("mode")] = amf0.NumberType(1.0)
			result.Object = ptr
			ptr = amf0.NewObjectType()
			obj = *ptr
			obj[amf0.StringType("level")] = amf0.StringType("status")
			obj[amf0.StringType("code")] = amf0.StringType("NetConnection.Connect.Success")
			obj[amf0.StringType("description")] = amf0.StringType("Connection succeeded")
			psubobj := amf0.NewObjectType()
			subobj := *psubobj
			subobj["version"] = amf0.StringType("3,5,1,525")
			obj[amf0.StringType("data")] = psubobj
			obj[amf0.StringType("objectEncoding")] = amf0.NumberType(0.0)
			result.Optional = ptr
			var err error
			message.Payload, err = EncodeCommand0(result)
			c.writeMessage(message)
			if err != nil {
				reply <- err
				continue
			}			
		} else {
			print("here2")
			result := new(Command)
			result.Name = "_error"
			result.Tid = cmd.Tid
			ptr := amf0.NewObjectType()
			obj := *ptr
			obj[amf0.StringType("fmsVer")] = amf0.StringType("FMS/3,5,1,525")
			obj[amf0.StringType("capabilities")] = amf0.NumberType(31)
			obj[amf0.StringType("mode")] = amf0.NumberType(1.0)
			result.Object = ptr
			ptr = amf0.NewObjectType()
			obj = *ptr
			obj[amf0.StringType("level")] = amf0.StringType("error")
			obj[amf0.StringType("code")] = amf0.StringType("NetConnection.Connect.InvalidApp")
			obj[amf0.StringType("description")] = amf0.StringType("invalid app")
			psubobj := amf0.NewObjectType()
			subobj := *psubobj
			subobj["version"] = amf0.StringType("3,5,1,525")
			obj[amf0.StringType("data")] = psubobj
			obj[amf0.StringType("objectEncoding")] = amf0.NumberType(0.0)
			result.Optional = ptr
			message.Type = AMF0Command
			var err error
			message.Payload, err = EncodeCommand0(result)
			c.writeMessage(message)
			if err != nil {
				reply <- err
				continue
			}			
		}
	}
}

type Server struct {
	mutex   sync.Mutex
	fs      *FlashServer
}

func NewServer(fs *FlashServer) *Server {
	return &Server{fs: fs}
}

func (s *Server) Serve(l net.Listener) error {
	go s.connectLoop()
	for {
		ln, err := l.Accept()
		// more to do with error
		if err != nil {
			return err
		}
		client := newClient(ln, s)
		go client.serve()
	}
	return nil
}

func (s *Server) ListenAndServe(laddr string) error {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return err
	}
	return s.Serve(l)
}
