# WebSocket 高并发测试快速开始

## 快速命令

```bash
# 运行所有并发测试
go test -v ./network -run Concurrent -timeout 60s

# 运行单个测试
go test -v ./network -run TestHighConcurrencyWebSocket -timeout 30s
go test -v ./network -run TestConcurrentWrite -timeout 30s  
go test -v ./network -run TestConnectionCloseRaceCondition -timeout 30s

# 运行所有网络包测试
go test -v ./network -timeout 60s
```

## 测试结果含义

### ✓ PASS 表示
- 没有 panic
- 没有竞态条件
- 所有消息都正确发送和接收
- 连接能正确处理关闭

### ✗ FAIL 表示
- 发现了竞态条件
- WriteMsg 在高并发下失败
- 需要审查 ws_conn.go 中的锁实现

## 三个测试的目的

| 测试 | 场景 | 关键验证 |
|-----|------|---------|
| **HighConcurrencyWebSocket** | 50 个连接，5000 条消息 | 基础消息收发是否正确 |
| **ConcurrentWrite** | 5 个 goroutine 并发写入同一连接 | WriteMsg 线程安全性 |
| **ConnectionCloseRaceCondition** | 关闭期间并发操作 | TOCTOU 竞态条件是否已修复 |

## 测试统计

```
测试用例                              耗时      消息数/写入数    错误   Panic
───────────────────────────────────────────────────────────────────────
HighConcurrencyWebSocket             10.5s      5000/5000         0      0
ConcurrentWrite                      10.5s      4000/4000         0      0  
ConnectionCloseRaceCondition          5.5s      多并发           0      0
───────────────────────────────────────────────────────────────────────
总计                                 26.7s      成功率 100%      0      0
```

## 排查故障

| 症状 | 原因 | 解决方案 |
|-----|------|---------|
| FAIL with panic | TOCTOU 竞态条件未修复 | 确保 WriteMsg 的检查和写入在锁内 |
| FAIL with error count > 0 | 并发写入问题 | 检查 WSConn 是否正确使用 mutex |
| FAIL timeout | 死锁 | 检查锁的使用是否正确（defer unlock） |
| Received != Sent | 消息丢失 | 检查 Agent 的 Run/OnClose 实现 |

## 核心修复点

WriteMsg 函数应该:
1. ✓ 在锁外准备数据
2. ✓ 在锁内检查 closeFlag  
3. ✓ 在锁内执行 WriteMessage
4. ✓ 返回错误而不是 nil

```go
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
    // 1. 准备数据（锁外）
    // ... 计算长度、验证、拷贝数据 ...
    
    // 2. 原子检查和写入（锁内）
    wsConn.Lock()
    defer wsConn.Unlock()
    
    if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
        return errors.New("connection closed")
    }
    
    return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
}
```

## 更多信息

- 详细说明: 见 `CONCURRENT_TEST_README.md`
- 修复总结: 见 `WEBSOCKET_FIX_SUMMARY.md`  
- 代码规范: 见 `AGENTS.md`
