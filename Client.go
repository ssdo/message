package message

type Client interface {
	Recv(msg *BaseMessage)
	RecvBulk(msg []*BaseMessage)
}
