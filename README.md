# transparent_proxy 应用
## 生产环境
```shell
go build -tags "pro" -ldflags="-H windowsgui -X main.version=version" -o bin/pro/transparent_proxy.exe  
```

## 预发布环境
```shell
go build -tags "uat" -ldflags=" -H windowsgui -X main.version=version" -o bin/uat/transparent_proxy.exe  
```

## 测试开发环境
```shell
go build -tags "dev" -ldflags=" -X main.version=version" -o bin/dev/transparent_proxy.exe  
```

## 代码格式化
```shell
 gofumpt -l -w .
```

 