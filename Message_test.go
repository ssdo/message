package message_test

import (
	"encoding/json"
	"github.com/ssdo/message"
	"github.com/ssgo/redis"
	"github.com/ssgo/u"
	"testing"
	"time"
)

type Client struct {
	id              string
	session         *message.Session
	lastMessage     *Message
	messageVersions string
	messageFrom     string
	messageTexts    string
}

type Message struct {
	Version uint64
	Time    int64
	From    string
	Text    string
}

func (msg *Message) Encode() []byte {
	buf, _ := json.Marshal(msg)
	return buf
}

func (msg *Message) Decode(data []byte) {
	_ = json.Unmarshal(data, msg)
}

func (client *Client) Send(text string) {
	msg := &Message{
		From: client.id,
		Text: text,
	}
	buf := msg.Encode()
	client.session.Send(buf)
}

func (client *Client) Recv(baseMsg *message.BaseMessage) {
	msg := &Message{Version: baseMsg.Version, Time: baseMsg.Time}
	msg.Decode(baseMsg.Data)
	msg.Version = baseMsg.Version
	msg.Time = baseMsg.Time
	client.messageVersions += u.String(baseMsg.Version)
	client.messageFrom += msg.From
	client.messageTexts += msg.Text
	client.lastMessage = msg
}

func (client *Client) RecvBulk(baseMessages []*message.BaseMessage) {
	for _, baseMsg := range baseMessages {
		if baseMsg != nil {
			client.Recv(baseMsg)
		}
	}
}

func TestMessage(t *testing.T) {
	rd := redis.GetRedis("test", nil)
	rd.DEL("_MSG_VER_s1", "_MSG_s1")

	rd.Start()
	message.Config.Redis = rd
	message.Init()

	sess := message.GetSession("s1")
	c1 := &Client{id: "c1", session: sess}
	c2 := &Client{id: "c2", session: sess}
	c3 := &Client{id: "c3", session: sess}
	sess.In("c1", c1, 0)
	sess.In("c2", c2, 0)

	c1.Send("hello1")
	c2.Send("hello2")
	c1.Send("hello3")
	c2.Send("hello4")
	c1.Send("hello5")

	time.Sleep(100 * time.Microsecond)

	if c1.messageVersions != "12345" || c2.messageVersions != "12345" || c1.messageFrom != "c1c2c1c2c1" || c2.messageFrom != "c1c2c1c2c1" || c1.messageTexts != "hello1hello2hello3hello4hello5" || c2.messageTexts != "hello1hello2hello3hello4hello5" {
		t.Fatal("failed to check 1")
	}

	c2.Send("hello6")
	c1.Send("hello7")

	time.Sleep(100 * time.Microsecond)

	if c1.messageVersions != "1234567" || c2.messageVersions != "1234567" {
		t.Fatal("failed to check 2", c1.messageVersions, c2.messageVersions)
	}

	sess.In("c3", c3, 0)

	time.Sleep(100 * time.Microsecond)

	if c3.messageVersions != "1234567" {
		t.Fatal("failed to check 3", c3.messageVersions)
	}

	c3.Send("hello8")

	time.Sleep(100 * time.Microsecond)

	if c1.messageVersions != "12345678" || c2.messageFrom != "c1c2c1c2c1c2c1c3" || c3.messageTexts != "hello1hello2hello3hello4hello5hello6hello7hello8" {
		t.Fatal("failed to check 4")
	}

	c1.Send("hello9")
	c2.Send("hello10")
	sess.Out("c2")
	c2.Send("hello11")

	time.Sleep(100 * time.Microsecond)

	if c1.messageVersions != "1234567891011" || c2.messageVersions != "12345678910" || c3.messageVersions != "1234567891011" {
		t.Fatal("failed to check 5", c2.messageVersions)
	}

	c1.Send("hello12")
	sess.In("c2", c2, c2.lastMessage.Version)
	c3.Send("hello13")

	time.Sleep(100 * time.Microsecond)

	if c1.messageVersions != "12345678910111213" || c2.messageVersions != "12345678910111213" || c3.messageVersions != "12345678910111213" {
		t.Fatal("failed to check 5", c2.messageVersions)
	}

	rd.Stop()
}
