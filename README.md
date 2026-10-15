# iotserver-tcp-driver

iotserver 物联网平台的 **TCP 协议驱动**：驱动监听一个 TCP 端口，设备用「按行分隔的 JSON」
报文接入，驱动转成平台物模型消息通过 gRPC 上报。

```
设备 ──TCP(JSON 行)──► 驱动 ──gRPC──► 平台核心
```

## 一、平台侧怎么用

1. **构建镜像**（换成你的仓库地址）：

   ```bash
   make push IMAGE=registry.example.com/iotserver-tcp-driver:v1.0.0
   ```

2. **平台「设备接入 → 驱动管理」**：添加驱动（镜像填上面的地址），
   到「驱动实例」新建实例，**自定义参数**填：

   ```json
   {
     "ListenAddr": "0.0.0.0:9000",
     "AutoCreateDevice": false
   }
   ```

3. **端口映射**：`ListenAddr` 的端口（默认 9000）是设备接入端口，
   在驱动实例的 Docker 参数里映射，例如 `-p 9000:9000`。

4. 平台「设备管理」新建设备并绑定该驱动实例，设备序列号（SN）用后面握手时发的那个。

## 二、设备侧怎么接

用任意 TCP 客户端连上 `ListenAddr`，按行发 JSON（`\n` 结尾）：

```jsonc
// 第 1 行：握手（必须），声明自己的设备序列号
{"sn": "device-001"}

// 之后：属性上报
{"temp": 23.5, "rpm": 1450}

// 事件上报
{"events": {"alarm": {"code": "E01", "msg": "超温"}}}
```

驱动对每一行回一条应答：

```jsonc
{"ok": true}                  // 成功
{"ok": false, "error": "..."} // 失败原因
```

### 下行（平台 → 设备）

平台下发属性设置 / 查询 / 服务调用时，驱动往同一个 TCP 连接写一行 JSON：

```jsonc
{"cmd": "set", "data": {"temp": 25}}
{"cmd": "get", "data": ["temp", "rpm"]}
{"cmd": "service", "data": {"code": "reboot", "inputParams": {}}}
```

### 联调（不用写代码，命令行就能测）

```bash
# 装 netcat 后直接手工发一帧
printf '{"sn":"device-001"}\n{"temp":23.5}\n' | nc <驱动主机> 9000
```

## 三、自定义参数

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `ListenAddr` | `0.0.0.0:9000` | TCP 监听地址 |
| `AutoCreateDevice` | `false` | 未注册设备连接时是否自动在平台创建设备 |
| `MaxFrameBytes` | `65536` | 单帧最大字节数 |

> 开发阶段可以开 `AutoCreateDevice` 省去手工建设备；生产建议关掉，
> 只允许平台已注册的设备接入。

## 四、协议设计说明

选「按行分隔的 JSON」而不是二进制私有协议，原因：

- 设备侧任何语言实现都只有几行，没有字节序 / 位域的心智负担；
- 出问题 `nc`/`telnet` 就能手工复现，不用抓包解协议；
- 平台侧物模型本来就是 JSON，省掉一层编解码。

如果设备协议是二进制（Modbus、私有帧），改动集中在
`internal/server/tcp_handler.go` 的 `readLine` + `internal/driver/driver.go` 的 `handleFrame`，
驱动框架（连接管理、平台上报、下行分发）不用动。

## 五、本地开发

```bash
go mod tidy
go build ./...
```

驱动依赖平台下发的配置（`/etc/driver/res/configuration.toml`），**不能脱离平台单独跑**。