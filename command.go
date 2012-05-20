package rtmp

import (
	"bytes"
	"errors"
	"github.com/hongruiqi/amf.go/amf0"
)

type Command struct {
	Name     string
	Tid      uint64
	Object   interface{}
	Optional interface{}
}

func DecodeCommand0(b []byte) (*Command, error) {
	cmd := new(Command)
	r := bytes.NewReader(b)
	dec := amf0.NewDecoder(r)
	v, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	if value, ok := v.(amf0.StringType); ok {
		cmd.Name = string(value)
	} else {
		return nil, errors.New("command Name decode error")
	}
	v, err = dec.Decode()
	if err != nil {
		return nil, err
	}
	if value, ok := v.(amf0.NumberType); ok {
		cmd.Tid = uint64(value)
	} else {
		return nil, errors.New("command Tid decode error")
	}
	v, err = dec.Decode()
	if err != nil {
		return nil, err
	}
	cmd.Object = v
	if r.Len() > 0 {
		v, err = dec.Decode()
		if err != nil {
			return nil, err
		}
		cmd.Optional = v
	}
	return cmd, nil
}

func EncodeCommand0(cmd *Command) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := amf0.NewEncoder(buf)
	err := enc.Encode(amf0.StringType(cmd.Name))
	if err != nil {
		return nil, err
	}
	err = enc.Encode(amf0.NumberType(cmd.Tid))
	if err != nil {
		return nil, err
	}
	err = enc.Encode(cmd.Object)
	if err != nil {
		return nil, err
	}
	if cmd.Object != nil {
		err = enc.Encode(cmd.Optional)
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
