package message

import (
	"github.com/ssgo/log"
	"github.com/ssgo/redis"
	"github.com/ssgo/u"
	"time"
)

var inited = false

var Config = struct {
	Redis            *redis.Redis // Redis连接池
	Logger           *log.Logger
	SessionIdMaker   func() string
	MessageAliveTime time.Duration
}{
	Redis:            nil,
	Logger:           log.DefaultLogger,
	SessionIdMaker:   u.Id8,
	MessageAliveTime: time.Hour * 4,
}

func checkInit() bool{
	if !inited {
		log.DefaultLogger.Error("message engine never init")
		return false
	}
	return true
}

func Init() {
	if inited {
		return
	}
	inited = true

	if Config.Redis == nil {
		Config.Redis = redis.GetRedis("user", Config.Logger)
	}

	if Config.Redis.SubRunning == false {
		log.DefaultLogger.Error("redis for message engine never start")
		//Config.Redis.Start()
	}
}
