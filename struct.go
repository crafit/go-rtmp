package rtmp

type Header struct {
	CurrentTime uint32
	Time        uint32
	Length      uint32
	Type        byte
	StreamId    uint32
	chunkId     uint16 // for write
}

type Payload []byte

func NewPayload(n uint32) Payload {
	return make(Payload, n)[0:0]
}

type Message struct {
	Header
	Payload
}

type chunk struct {
	Type byte
	Header
	Payload
}
