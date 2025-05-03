package lua

import (
	"context"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"proxy_server/common"
	"proxy_server/log"
)

const SetUserDataLuaScript = `
local stringKey = KEYS[1]
local setKey = KEYS[2]

-- 从 ARGV 数组中获取字符串值和集合元素
local stringValue = ARGV[1]
local setMembers = {}
for i = 2, #ARGV do
    setMembers[i - 1] = ARGV[i]
end

-- 设置字符串
redis.call('SET', stringKey, stringValue)

-- 如果集合存在，先删除原集合
redis.call('DEL', setKey)

redis.call('SADD', setKey, unpack(setMembers))

-- 返回成功标识
return 1
`

var SetUserDataLuaScriptShaCode string

func init() {
	// 加载 Lua 脚本
	script := redis.NewScript(SetUserDataLuaScript)
	sha, err := script.Load(context.Background(), common.GetRedisDB()).Result()
	if err != nil {
		log.Panic("[lua] 加载SetUserDataLuaScript失败", zap.Error(err))
	}
	SetUserDataLuaScriptShaCode = sha
}
