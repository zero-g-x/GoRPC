package codec

import "io"
/*
RCP call: err=client.Call("[service name].[method name]",args,&reply)
*/
type Header struct {
	ServiceMethod string // service and method name,format [service name].[method name]
	Seq           uint64 //sequence number chosen by client
	Error         string // nil for client
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

//return a construct function of Codec
type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

const (
	GobType  Type = "application/gob"
	JsonType Type = "application/json" //TODO
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}
