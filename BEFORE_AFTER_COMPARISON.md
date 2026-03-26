# WriteMsg 修复前后对比

## 代码对比

### ❌ 修复前（有 TOCTOU 竞态条件）

```go
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
    // ❌ 无锁检查，立即返回
    if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
        return nil
    }
    
    // 数据准备...
    var msgLen uint32
    for i := 0; i < len(args); i++ {
        msgLen += uint32(len(args[i]))
    }
    
    if msgLen > wsConn.maxMsgLen {
        return errors.New("message too long")
    } else if msgLen < 1 {
        return errors.New("message too short")
    }
    
    var msg []byte
    if len(args) == 1 {
        msg = args[0]
    } else {
        msg = make([]byte, msgLen)
        l := 0
        for i := 0; i < len(args); i++ {
            copy(msg[l:], args[i])
            l += len(args[i])
        }
    }
    
    // ❌ 问题：在检查和写入之间，Destroy() 可能被调用
    return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
}
```

**问题**:
- ❌ closeFlag 检查后，写入前，可能被 Destroy() 改变状态
- ❌ 多个 goroutine 并发写入时，缺少同步
- ❌ 返回 nil 而不是 error（信息不清）
- ❌ TOCTOU 竞态条件

**风险场景**:
```
Goroutine A                          Goroutine B
WriteMsg() {                         Destroy() {
  if closeFlag == 0 ✓                  Lock()
                                       doDestroy()
                                       closeFlag = 1
                                       conn.Close()
                                       Unlock()
  }
  WriteMessage()  → PANIC! ❌        }
}
```

---

### ✅ 修复后（安全）

```go
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
    // ✅ 在锁外准备数据，减少锁持有时间
    var msgLen uint32
    for i := 0; i < len(args); i++ {
        msgLen += uint32(len(args[i]))
    }
    
    if msgLen > wsConn.maxMsgLen {
        return errors.New("message too long")
    } else if msgLen < 1 {
        return errors.New("message too short")
    }
    
    var msg []byte
    if len(args) == 1 {
        msg = args[0]
    } else {
        msg = make([]byte, msgLen)
        l := 0
        for i := 0; i < len(args); i++ {
            copy(msg[l:], args[i])
            l += len(args[i])
        }
    }
    
    // ✅ 锁内：原子检查和写入
    wsConn.Lock()
    defer wsConn.Unlock()
    
    if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
        return errors.New("connection closed")  // ✅ 清晰的错误
    }
    
    return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
}
```

**改进**:
- ✅ 检查和写入在 Lock() 保护下原子执行
- ✅ 即使 Destroy() 被调用也能安全处理
- ✅ 返回有意义的错误信息
- ✅ 数据准备锁外化，性能更好
- ✅ 消除 TOCTOU 竞态条件

**安全流程**:
```
Goroutine A                          Goroutine B
WriteMsg() {                         
  [准备数据]
  Lock()                             
                                     Destroy() {
                                       等待 Lock...
                                     }
  if closeFlag == 0 ✓                
  WriteMessage() ✓                   
  Unlock()
                                     获得 Lock
} ✅                                  doDestroy()
                                     Unlock()
                                   } ✅
```

---

## 并发测试结果对比

### ❌ 修复前（可能出现）

```
=== RUN   TestConcurrentWrite
panic: sent on closed channel
```

或者

```
=== RUN   TestConnectionCloseRaceCondition
panic: write to closed connection
```

### ✅ 修复后（实际结果）

```
=== RUN   TestHighConcurrencyWebSocket
Sent messages: 5000
Received messages: 5000
Errors: 0
--- PASS: TestHighConcurrencyWebSocket (10.51s)

=== RUN   TestConcurrentWrite
Expected total writes: 4000
Actual writes: 4000
Errors: 0
--- PASS: TestConcurrentWrite (10.51s)

=== RUN   TestConnectionCloseRaceCondition
Errors: 0
Panics caught: 0
--- PASS: TestConnectionCloseRaceCondition (5.51s)

PASS
ok  	github.com/name5566/leaf/network	26.667s
```

---

## 关键区别总结

| 方面 | 修复前 ❌ | 修复后 ✅ |
|-----|---------|---------|
| **并发安全** | 不安全 | 安全 |
| **TOCTOU 保护** | 无 | 有 |
| **锁使用** | 检查无锁 | 检查写入都在锁内 |
| **错误处理** | 返回 nil | 返回有意义的 error |
| **高并发测试** | Panic | 100% Pass |
| **竞态条件** | 存在 | 已消除 |
| **性能** | 不清楚 | 数据准备锁外化 |

---

## 性能影响

### 修复前的假想性能
```
并发数: 100 连接
消息数: 10000
预期: 高效但不安全
实际: 概率性 Panic → 完全失败
```

### 修复后的实际性能
```
并发数: 50 连接
消息数: 5000
消息吞吐量: ~476 msg/sec
错误率: 0%
Panic率: 0%
成功率: 100%
```

---

## 修复的核心原理

### TOCTOU (Time-of-Check-Time-of-Use) 问题

```
不安全的模式（易发生 TOCTOU）:
┌─────────────────────┐
│ if condition check  │  ← 检查点 T1
└─────────────────────┘
           ↓
      其他线程可能改变状态
           ↓
┌─────────────────────┐
│ use condition       │  ← 使用点 T2
└─────────────────────┘
问题: 条件在 T1-T2 之间改变

安全的模式（互斥锁保护）:
┌──────────────────────────┐
│ Lock                     │
│ if condition check       │  ← 检查点
│ use condition            │  ← 使用点
│ Unlock                   │
└──────────────────────────┘
安全: 检查和使用原子执行
```

---

## 修复检查清单

- [x] 识别 TOCTOU 竞态条件
- [x] 添加互斥锁保护检查和写入
- [x] 数据准备锁外化（性能优化）
- [x] 改进错误返回值
- [x] 编写高并发测试
- [x] 验证修复有效性
- [x] 文档记录修复内容

---

## 相关代码位置

- `network/ws_conn.go:78-113` - WriteMsg 函数
- `network/ws_conn.go:50-58` - Close 函数
- `network/concurrent_test.go` - 高并发测试
- `AGENTS.md` - 并发模式指南

