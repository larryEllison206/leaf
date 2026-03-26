package network

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHighConcurrencyWebSocket 测试高并发情况下 WebSocket 连接的收发
func TestHighConcurrencyWebSocket(t *testing.T) {
	// 配置参数
	const (
		numConnections  = 50   // 并发连接数
		messagesPerConn = 100  // 每个连接发送的消息数
		maxMsgLen       = 4096 // 最大消息长度
		serverAddr      = "127.0.0.1:18888"
	)

	// 统计信息
	var (
		sentMsgCount     int64
		receivedMsgCount int64
		errorCount       int64
	)

	// 创建服务器
	server := &WSServer{
		Addr:            serverAddr,
		MaxConnNum:      numConnections * 2,
		PendingWriteNum: 100,
		MaxMsgLen:       maxMsgLen,
		HTTPTimeout:     10 * time.Second,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestAgent{
				conn:             wsConn,
				receivedMsgCount: &receivedMsgCount,
				errorCount:       &errorCount,
			}
		},
	}
	server.Start()
	defer server.Close()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 创建客户端
	client := &WSClient{
		Addr:             "ws://" + serverAddr,
		ConnNum:          numConnections,
		ConnectInterval:  100 * time.Millisecond,
		PendingWriteNum:  100,
		MaxMsgLen:        maxMsgLen,
		HandshakeTimeout: 10 * time.Second,
		AutoReconnect:    false,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestClientAgent{
				conn:            wsConn,
				messagesPerConn: messagesPerConn,
				sentMsgCount:    &sentMsgCount,
				errorCount:      &errorCount,
			}
		},
	}
	client.Start()
	defer client.Close()

	// 等待所有连接完成
	time.Sleep(10 * time.Second)

	// 输出结果
	fmt.Printf("=== High Concurrency WebSocket Test Results ===\n")
	fmt.Printf("Number of connections: %d\n", numConnections)
	fmt.Printf("Messages per connection: %d\n", messagesPerConn)
	fmt.Printf("Expected total messages: %d\n", numConnections*messagesPerConn)
	fmt.Printf("Sent messages: %d\n", atomic.LoadInt64(&sentMsgCount))
	fmt.Printf("Received messages: %d\n", atomic.LoadInt64(&receivedMsgCount))
	fmt.Printf("Errors: %d\n", atomic.LoadInt64(&errorCount))

	// 验证结果
	expectedMessages := int64(numConnections * messagesPerConn)
	sentMessages := atomic.LoadInt64(&sentMsgCount)
	receivedMessages := atomic.LoadInt64(&receivedMsgCount)
	errors := atomic.LoadInt64(&errorCount)

	if errors > 0 {
		t.Fatalf("Test failed with %d errors", errors)
	}

	if sentMessages != expectedMessages {
		t.Logf("Warning: Sent %d messages, expected %d", sentMessages, expectedMessages)
	}

	if receivedMessages != expectedMessages {
		t.Logf("Warning: Received %d messages, expected %d", receivedMessages, expectedMessages)
	}

	fmt.Printf("\nTest completed successfully!\n")
}

// TestConcurrentWrite 测试 WriteMsg 并发写入
func TestConcurrentWrite(t *testing.T) {
	const (
		numConnections = 20
		writesPerConn  = 200
		maxMsgLen      = 4096
		serverAddr     = "127.0.0.1:18889"
	)

	var (
		writeCount int64
		errorCount int64
	)

	server := &WSServer{
		Addr:            serverAddr,
		MaxConnNum:      numConnections * 2,
		PendingWriteNum: 100,
		MaxMsgLen:       maxMsgLen,
		HTTPTimeout:     10 * time.Second,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestEchoAgent{
				conn:       wsConn,
				errorCount: &errorCount,
			}
		},
	}
	server.Start()
	defer server.Close()

	time.Sleep(500 * time.Millisecond)

	client := &WSClient{
		Addr:             "ws://" + serverAddr,
		ConnNum:          numConnections,
		ConnectInterval:  100 * time.Millisecond,
		PendingWriteNum:  100,
		MaxMsgLen:        maxMsgLen,
		HandshakeTimeout: 10 * time.Second,
		AutoReconnect:    false,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestConcurrentWriteAgent{
				conn:        wsConn,
				writesCount: writesPerConn,
				writeCount:  &writeCount,
				errorCount:  &errorCount,
			}
		},
	}
	client.Start()
	defer client.Close()

	time.Sleep(10 * time.Second)

	fmt.Printf("=== Concurrent Write Test Results ===\n")
	fmt.Printf("Number of connections: %d\n", numConnections)
	fmt.Printf("Writes per connection: %d\n", writesPerConn)
	fmt.Printf("Expected total writes: %d\n", numConnections*writesPerConn)
	fmt.Printf("Actual writes: %d\n", atomic.LoadInt64(&writeCount))
	fmt.Printf("Errors: %d\n", atomic.LoadInt64(&errorCount))

	if atomic.LoadInt64(&errorCount) > 0 {
		t.Fatalf("Test failed with %d errors", atomic.LoadInt64(&errorCount))
	}

	fmt.Printf("\nConcurrent write test completed successfully!\n")
}

// TestConnectionCloseRaceCondition 测试关闭期间的竞态条件
func TestConnectionCloseRaceCondition(t *testing.T) {
	const (
		numConnections = 30
		maxMsgLen      = 4096
		serverAddr     = "127.0.0.1:18890"
	)

	var (
		errorCount int64
		panicCount int64
	)

	server := &WSServer{
		Addr:            serverAddr,
		MaxConnNum:      numConnections * 2,
		PendingWriteNum: 100,
		MaxMsgLen:       maxMsgLen,
		HTTPTimeout:     10 * time.Second,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestRaceAgent{
				conn:       wsConn,
				errorCount: &errorCount,
				panicCount: &panicCount,
			}
		},
	}
	server.Start()
	defer server.Close()

	time.Sleep(500 * time.Millisecond)

	client := &WSClient{
		Addr:             "ws://" + serverAddr,
		ConnNum:          numConnections,
		ConnectInterval:  100 * time.Millisecond,
		PendingWriteNum:  100,
		MaxMsgLen:        maxMsgLen,
		HandshakeTimeout: 10 * time.Second,
		AutoReconnect:    false,
		NewAgent: func(wsConn *WSConn) Agent {
			return &TestRaceClientAgent{
				conn:       wsConn,
				errorCount: &errorCount,
				panicCount: &panicCount,
			}
		},
	}
	client.Start()
	defer client.Close()

	time.Sleep(5 * time.Second)

	fmt.Printf("=== Connection Close Race Condition Test ===\n")
	fmt.Printf("Number of connections: %d\n", numConnections)
	fmt.Printf("Errors: %d\n", atomic.LoadInt64(&errorCount))
	fmt.Printf("Panics caught: %d\n", atomic.LoadInt64(&panicCount))

	if atomic.LoadInt64(&panicCount) > 0 {
		t.Fatalf("Test failed with %d panics", atomic.LoadInt64(&panicCount))
	}

	fmt.Printf("\nRace condition test completed successfully!\n")
}

// ========== Test Agent Implementations ==========

// TestAgent 服务端测试 agent，回显收到的消息
type TestAgent struct {
	conn             *WSConn
	receivedMsgCount *int64
	errorCount       *int64
}

func (a *TestAgent) Run() {
	for {
		b, err := a.conn.ReadMsg()
		if err != nil {
			break
		}

		atomic.AddInt64(a.receivedMsgCount, 1)

		// 回复确认消息
		if err := a.conn.WriteMsg([]byte("ACK"), b); err != nil {
			atomic.AddInt64(a.errorCount, 1)
			break
		}
	}
}

func (a *TestAgent) OnClose() {}

// TestClientAgent 客户端测试 agent，发送消息并接收回复
type TestClientAgent struct {
	conn            *WSConn
	messagesPerConn int
	sentMsgCount    *int64
	errorCount      *int64
}

func (a *TestClientAgent) Run() {
	for i := 0; i < a.messagesPerConn; i++ {
		// 生成随机消息
		msg := make([]byte, rand.Intn(256)+64)
		rand.Read(msg)

		if err := a.conn.WriteMsg(msg); err != nil {
			atomic.AddInt64(a.errorCount, 1)
			return
		}
		atomic.AddInt64(a.sentMsgCount, 1)

		// 接收回复
		if _, err := a.conn.ReadMsg(); err != nil {
			atomic.AddInt64(a.errorCount, 1)
			return
		}
	}
}

func (a *TestClientAgent) OnClose() {}

// TestEchoAgent 服务端回显 agent
type TestEchoAgent struct {
	conn       *WSConn
	errorCount *int64
}

func (a *TestEchoAgent) Run() {
	for {
		b, err := a.conn.ReadMsg()
		if err != nil {
			break
		}

		if err := a.conn.WriteMsg(b); err != nil {
			atomic.AddInt64(a.errorCount, 1)
			break
		}
	}
}

func (a *TestEchoAgent) OnClose() {}

// TestConcurrentWriteAgent 客户端并发写入 agent
type TestConcurrentWriteAgent struct {
	conn        *WSConn
	writesCount int
	writeCount  *int64
	errorCount  *int64
}

func (a *TestConcurrentWriteAgent) Run() {
	var wg sync.WaitGroup

	// 创建多个 goroutine 从同一连接写入
	numWriters := 5
	writesPerWriter := a.writesCount / numWriters

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := []byte("test message")
			for i := 0; i < writesPerWriter; i++ {
				if err := a.conn.WriteMsg(msg); err != nil {
					atomic.AddInt64(a.errorCount, 1)
					return
				}
				atomic.AddInt64(a.writeCount, 1)
			}
		}()
	}

	wg.Wait()

	// 关闭连接前读取所有回复
	for {
		_, err := a.conn.ReadMsg()
		if err != nil {
			break
		}
	}
}

func (a *TestConcurrentWriteAgent) OnClose() {}

// TestRaceAgent 服务端竞态条件测试 agent
type TestRaceAgent struct {
	conn       *WSConn
	errorCount *int64
	panicCount *int64
}

func (a *TestRaceAgent) Run() {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(a.panicCount, 1)
			fmt.Printf("Server panic caught: %v\n", r)
		}
	}()

	for {
		b, err := a.conn.ReadMsg()
		if err != nil {
			break
		}

		// 在随机延迟后回复，增加竞态条件的概率
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

		if err := a.conn.WriteMsg(b); err != nil {
			break
		}
	}
}

func (a *TestRaceAgent) OnClose() {}

// TestRaceClientAgent 客户端竞态条件测试 agent
type TestRaceClientAgent struct {
	conn       *WSConn
	errorCount *int64
	panicCount *int64
}

func (a *TestRaceClientAgent) Run() {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(a.panicCount, 1)
			fmt.Printf("Client panic caught: %v\n", r)
		}
	}()

	// 创建多个 goroutine 并发发送消息
	var wg sync.WaitGroup
	numSenders := 10

	for s := 0; s < numSenders; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(a.panicCount, 1)
					fmt.Printf("Client sender panic: %v\n", r)
				}
			}()

			msg := []byte("race test")
			for i := 0; i < 50; i++ {
				if err := a.conn.WriteMsg(msg); err != nil {
					return
				}
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}()
	}

	// 读取回复
	go func() {
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(a.panicCount, 1)
			}
		}()
		for {
			_, err := a.conn.ReadMsg()
			if err != nil {
				return
			}
		}
	}()

	wg.Wait()
	time.Sleep(100 * time.Millisecond)
}

func (a *TestRaceClientAgent) OnClose() {}
