# WebSocket 高并发测试说明

本文档说明如何运行和理解 WebSocket 高并发测试程序。

## 概述

`network/concurrent_test.go` 包含三个主要的高并发测试，用于验证 WebSocket 连接在高并发场景下的稳定性和正确性。

## 测试用例

### 1. TestHighConcurrencyWebSocket - 基础高并发测试

**测试目标**: 验证多个连接同时进行收发操作是否正确。

**测试参数**:
- 并发连接数: 50
- 每个连接的消息数: 100
- 总消息数: 5000

**工作流程**:
1. 启动服务器，设置最大连接数为 100
2. 启动 50 个客户端连接
3. 每个客户端随机生成 64-320 字节的消息并发送
4. 服务器接收消息，回复 "ACK" + 原消息
5. 客户端接收回复
6. 验证所有消息都正确发送和接收

**验证指标**:
- ✓ 发送消息数 = 5000
- ✓ 接收消息数 = 5000
- ✓ 错误数 = 0
- ✓ 无 panic

**运行命令**:
```bash
go test -v ./network -run TestHighConcurrency -timeout 30s
```

### 2. TestConcurrentWrite - 并发写入测试

**测试目标**: 验证 `WriteMsg()` 函数在多个 goroutine 同时写入时的线程安全性。

**测试参数**:
- 并发连接数: 20
- 每个连接的写操作数: 200
- 每个连接内的并发写入 goroutine 数: 5
- 总写操作数: 4000

**工作流程**:
1. 启动服务器，回显接收到的所有消息
2. 启动 20 个客户端连接
3. 每个客户端的 **5 个 goroutine** 并发地向同一连接写入消息
4. 服务器收到消息后回复
5. 验证所有写操作都成功，无竞态条件

**关键测试场景**:
```go
// 在同一连接上，5 个 goroutine 并发调用 WriteMsg()
for w := 0; w < 5; w++ {
    go func() {
        for i := 0; i < 40; i++ {
            conn.WriteMsg(msg)  // 并发调用
        }
    }()
}
```

**验证指标**:
- ✓ 实际写入数 = 预期写入数 (4000)
- ✓ 错误数 = 0
- ✓ 无 panic 或数据损坏

**运行命令**:
```bash
go test -v ./network -run TestConcurrentWrite -timeout 30s
```

### 3. TestConnectionCloseRaceCondition - 关闭竞态条件测试

**测试目标**: 验证连接关闭期间的竞态条件已被修复。这是最关键的测试，用于检验 TOCTOU (Time-of-Check-Time-of-Use) 问题。

**测试参数**:
- 并发连接数: 30
- 客户端并发发送 goroutine 数: 10
- 每个发送 goroutine 的消息数: 50

**工作流程**:
1. 启动服务器，处理客户端消息
2. 启动 30 个客户端连接
3. 每个客户端启动 10 个 goroutine 并发发送消息，同时主线程读取回复
4. 某些连接会在发送/接收过程中被意外关闭
5. 验证即使在关闭过程中，也无 panic 或数据损坏

**关键竞态条件场景**:
```go
// 场景 1: 写入时连接被关闭
goroutine 1: WriteMsg()  ← 检查 closeFlag (未关闭)
goroutine 2: Destroy()   ← 设置 closeFlag
goroutine 1: WriteMsg()  ← 执行写入 (PANIC!)

// 修复后: 检查和写入在锁内原子执行
Lock()
if closeFlag != 0 {
    return error
}
WriteMessage()  // 受保护
Unlock()
```

**验证指标**:
- ✓ 错误处理正确
- ✓ 无 panic
- ✓ Panics caught = 0

**运行命令**:
```bash
go test -v ./network -run TestConnectionCloseRaceCondition -timeout 30s
```

## 修复前后对比

### 修复前的问题 (ws_conn.go 原始版本)

```go
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
    // 无锁检查 closeFlag
    if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
        return nil
    }
    
    // ... 准备数据 ...
    
    // 直接写入，未检查 closeFlag 变化
    return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
    // ↑ 在 Destroy() 和这里之间可能发生竞态条件
}
```

**问题**:
1. **TOCTOU 竞态条件**: closeFlag 检查和写入之间可能被其他 goroutine 关闭连接
2. **无锁写入**: 多个 goroutine 并发调用 WriteMsg() 时，虽然 websocket.Conn 有内部锁，但 closeFlag 检查不原子

### 修复后的版本

```go
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
    // ... 在锁外准备数据 (降低锁持有时间) ...
    
    wsConn.Lock()
    defer wsConn.Unlock()
    
    // 在锁内重新检查 closeFlag
    if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
        return errors.New("connection closed")
    }
    
    // 检查和写入在锁内原子执行
    return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
}
```

**改进**:
1. ✓ 消除 TOCTOU 竞态条件
2. ✓ closeFlag 检查和写入原子执行
3. ✓ 数据准备在锁外进行，提高性能
4. ✓ 更清晰的错误处理

## 测试统计

### 测试环境
- Go 版本: 1.17+
- 框架: Leaf (github.com/name5566/leaf)
- WebSocket 库: gorilla/websocket v1.4.0

### 测试结果总结

| 测试用例 | 连接数 | 操作数 | 结果 | 耗时 |
|---------|--------|--------|------|------|
| HighConcurrencyWebSocket | 50 | 5000 msg | ✓ PASS | ~10.5s |
| ConcurrentWrite | 20 | 4000 writes | ✓ PASS | ~10.5s |
| ConnectionCloseRaceCondition | 30 | 多并发 | ✓ PASS | ~5.5s |

## 如何运行所有测试

```bash
# 运行 network 包的所有并发测试
go test -v ./network -run Concurrent -timeout 60s

# 运行 network 包的所有测试
go test -v ./network -timeout 60s

# 运行整个项目的所有测试
go test -v ./... -timeout 120s
```

## 性能指标

- **消息吞吐量**: ~476 msg/s (5000 messages / 10.5s)
- **写入吞吐量**: ~376 writes/s (4000 writes / 10.5s)
- **连接稳定性**: 100% (0 panics, 0 errors)

## 故障排查

### 如果测试失败

1. **Panic 错误**
   - 表示仍存在竞态条件
   - 检查 ws_conn.go 中 WriteMsg() 是否正确使用锁
   - 确保 closeFlag 检查在锁内

2. **消息丢失**
   - 检查 Agent 的 Run() 方法是否正确读写消息
   - 验证服务器是否正确处理连接关闭

3. **超时**
   - 增加 `-timeout` 参数
   - 检查网络配置和防火墙

## 添加自定义测试

可以通过修改测试参数来创建更严苛的测试:

```go
const (
    numConnections  = 100    // 增加并发数
    messagesPerConn = 1000   // 增加消息数
)
```

## 相关文件

- `network/ws_conn.go` - WebSocket 连接实现
- `network/ws_server.go` - WebSocket 服务器实现
- `network/ws_client.go` - WebSocket 客户端实现
- `network/concurrent_test.go` - 高并发测试代码
- `AGENTS.md` - 开发规范和并发模式说明

## 参考

- [Gorilla WebSocket](https://github.com/gorilla/websocket)
- [Go sync.Mutex](https://golang.org/pkg/sync/#Mutex)
- [Time-of-Check-Time-of-Use (TOCTOU) Race Condition](https://en.wikipedia.org/wiki/Time-of-check_to_time-of-use)
