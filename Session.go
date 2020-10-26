package message

import (
	"encoding/binary"
	"github.com/ssgo/u"
	"sync"
	"time"
)

type Session struct {
	id                string
	lastUsedTime      int64
	clients           map[string]Client
	clientsLock       sync.Mutex
	messages          map[uint64]*BaseMessage
	messageMinVersion uint64
	messageMaxVersion uint64
	messagesLock      sync.Mutex
	channelKey        string
	messagesKey       string
	minVerKey         string
	maxVerKey         string
}

var sessions sync.Map

// 创建一个新的会话
func NewSession() (string, *Session) {
	if !checkInit() {
		return "", nil
	}
	id := ""
	for i := 0; i < 10000; i++ {
		id = Config.SessionIdMaker()
		if !Config.Redis.EXISTS(id) {
			break
		}
	}
	return id, GetSession(id)
}

// 得到一个已经存在的会话
func GetSession(id string) *Session {
	if !checkInit() {
		return nil
	}
	obj, ok := sessions.Load(id)
	var sess *Session
	if !ok {
		sess = &Session{
			id:                id,
			lastUsedTime:      time.Now().UnixNano(),
			clients:           map[string]Client{},
			clientsLock:       sync.Mutex{},
			messages:          map[uint64]*BaseMessage{},
			messageMinVersion: 0,
			messageMaxVersion: 0,
			messagesLock:      sync.Mutex{},
			channelKey:        "_MSG_CH_" + id,
			messagesKey:       "_MSG_" + id,
			minVerKey:         "_MSG_MIN_VER_" + id,
			maxVerKey:         "_MSG_VER_" + id,
		}
		Config.Redis.Subscribe(sess.channelKey, sess.reset, sess.received)
	} else {
		sess = obj.(*Session)
	}
	return sess
}

// 关闭
func (sess *Session) Close() {
	Config.Redis.Unsubscribe(sess.channelKey)
}

// Redis断线时重新获取数据
func (sess *Session) reset() {
	rd := Config.Redis

	sess.messagesLock.Lock()
	// 计算需要从哪个版本开始拉取数据
	var startVer uint64
	if sess.messageMaxVersion > 0 {
		startVer = sess.messageMaxVersion
	} else {
		startVer = rd.GET(sess.minVerKey).Uint64()
	}
	remoteMaxVer := rd.GET(sess.maxVerKey).Uint64()

	// 一次性获取所有需要的消息
	fields := make([]string, remoteMaxVer-startVer)
	versions := make([]uint64, remoteMaxVer-startVer)
	i := 0
	for ver := startVer+1; ver <= remoteMaxVer; ver++ {
		fields[i] = u.String(ver)
		versions[i] = ver
		i++
	}
	results := rd.HMGET(sess.messagesKey, fields...)

	// 处理所有收到的消息
	bulkMessages := make([]*BaseMessage, len(results))
	for i, r := range results {
		buf := r.Bytes()
		if len(buf) > 8 {
			ver := versions[i]
			msg := makeBaseMessage(ver, buf)
			if sess.messageMinVersion == 0 {
				sess.messageMinVersion = ver
			}
			sess.messages[ver] = msg
			bulkMessages[i] = msg
		}
	}
	sess.messageMaxVersion = remoteMaxVer

	// 将数据发送给在线的客户端
	sess.clientsLock.Lock()
	for _, client := range sess.clients {
		client.RecvBulk(bulkMessages)
	}
	sess.clientsLock.Unlock()

	sess.messagesLock.Unlock()
}

// 接收到推送
func (sess *Session) received(data []byte) {
	if len(data) < 16 {
		return
	}

	// 读取版本号和时间戳
	version := binary.LittleEndian.Uint64(data[0:8])
	msg := makeBaseMessage(version, data[8:])

	//if version != sess.messageMaxVersion+1 {
	//	// TODO 控制顺序
	//	sess.pendingMessages = append(sess.pendingMessages, msg)
	//	return
	//}

	sess.dispatch(msg)

	// TODO 如果有暂存的消息
	//if len(sess.pendingMessages) > 0 {
	//	// .........
	//}
}

func (sess *Session) dispatch(msg *BaseMessage) {
	// 将数据存入队列
	sess.messagesLock.Lock()
	sess.messages[msg.Version] = msg
	sess.messageMaxVersion = msg.Version
	if sess.messageMinVersion == 0 {
		sess.messageMinVersion = msg.Version
	}
	sess.messagesLock.Unlock()

	// 将数据直接发送给在线的客户端
	sess.clientsLock.Lock()
	for _, client := range sess.clients {
		client.Recv(msg)
	}
	sess.clientsLock.Unlock()
}

func makeBaseMessage(version uint64, data []byte) *BaseMessage {
	tm := int64(binary.LittleEndian.Uint64(data[0:8]))
	data = data[8:]
	return &BaseMessage{
		Version: version,
		Time:    tm,
		Data:    data,
	}
}

// 发送消息
func (sess *Session) Send(data []byte) {
	rd := Config.Redis
	version := uint64(rd.INCR(sess.maxVerKey))
	tm := time.Now().UnixNano()
	versionBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(versionBuf, version)
	timeBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(timeBuf, uint64(tm))
	buf := make([]byte, 16+len(data))
	copy(buf, versionBuf)
	copy(buf[8:], timeBuf)
	copy(buf[16:], data)
	rd.HSET(sess.messagesKey, u.String(version), buf[8:])
	rd.Do("PUBLISH", sess.channelKey, buf)
}

// 客户端进入
func (sess *Session) In(id string, client Client, startVersion uint64) {
	sess.messagesLock.Lock()
	messages := make([]*BaseMessage, sess.messageMaxVersion-startVersion)
	i := 0
	for ver := startVersion+1; ver <= sess.messageMaxVersion; ver++ {
		messages[i] = sess.messages[ver]
		i++
	}
	sess.messagesLock.Unlock()

	sess.clientsLock.Lock()
	sess.clients[id] = client
	client.RecvBulk(messages)
	sess.clientsLock.Unlock()
}

// 客户端退出
func (sess *Session) Out(id string) {
	sess.clientsLock.Lock()
	delete(sess.clients, id)
	if len(sess.clients) == 0 {
		sessions.Delete(sess.id)
		sess.Close()
	}
	sess.clientsLock.Unlock()
}
