package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"proxy_server/common"
	"proxy_server/protobuf"
)

func (m *manager) Valid(ctx context.Context, username, password, ipAddr string) (authInfo *protobuf.AuthInfo, resErr error) {
	storeKey := fmt.Sprint(username, ":Auth")
	str, err := common.GetRedisDB().Get(ctx, storeKey).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("鉴权失败 error:%+v", err)
	} else {
		authInfo = &protobuf.AuthInfo{}
		err := json.Unmarshal([]byte(str), authInfo)
		if err != nil {
			return nil, fmt.Errorf("鉴权失败 error:%+v", err)
		}
	}

	if authInfo.Username != username || authInfo.Password != password {
		return nil, fmt.Errorf("密码错误")
	}

	if _, ok := authInfo.Ips[ipAddr]; !ok {
		return nil, fmt.Errorf("ip地址错误")
	}

	return authInfo, nil
}
