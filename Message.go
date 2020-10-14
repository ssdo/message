package message

type BaseMessage struct {
	Version uint64
	Time    int64
	Data    []byte
}

type Message interface {
	Encode() []byte
	Decode(data []byte)
}
