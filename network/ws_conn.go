package network

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

type WebsocketConnSet map[*websocket.Conn]struct{}

type WSConn struct {
	sync.Mutex
	conn           *websocket.Conn
	maxMsgLen      uint32
	closeFlag      int32
	remoteOriginIP net.Addr
}

func newWSConn(conn *websocket.Conn, pendingWriteNum int, maxMsgLen uint32) *WSConn {
	wsConn := new(WSConn)
	wsConn.conn = conn
	wsConn.maxMsgLen = maxMsgLen

	return wsConn
}

func (wsConn *WSConn) SetOriginIP(ip net.Addr) {
	wsConn.remoteOriginIP = ip
}

func (wsConn *WSConn) doDestroy() {
	if tcpConn, ok := wsConn.conn.UnderlyingConn().(*net.TCPConn); ok {
		tcpConn.SetLinger(0)
	}
	wsConn.conn.Close()

	atomic.StoreInt32(&wsConn.closeFlag, 1)
}

func (wsConn *WSConn) Destroy() {
	wsConn.Lock()
	defer wsConn.Unlock()

	wsConn.doDestroy()
}

func (wsConn *WSConn) Close() {
	wsConn.Lock()
	defer wsConn.Unlock()
	if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
		return
	}

	wsConn.doDestroy()
}

func (wsConn *WSConn) LocalAddr() net.Addr {
	return wsConn.conn.LocalAddr()
}

func (wsConn *WSConn) RemoteAddr() net.Addr {
	if wsConn.remoteOriginIP != nil {
		return wsConn.remoteOriginIP
	}
	return wsConn.conn.RemoteAddr()
}

// goroutine not safe
func (wsConn *WSConn) ReadMsg() ([]byte, error) {
	_, b, err := wsConn.conn.ReadMessage()
	return b, err
}

// args must not be modified by the others goroutines
func (wsConn *WSConn) WriteMsg(args ...[]byte) error {
	// 计算长度（锁外）
	var msgLen uint32
	for i := 0; i < len(args); i++ {
		msgLen += uint32(len(args[i]))
	}

	// 长度检查（锁外）
	if msgLen > wsConn.maxMsgLen {
		return errors.New("message too long")
	} else if msgLen < 1 {
		return errors.New("message too short")
	}

	// 准备数据（锁外）
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

	// 上锁：再次检查 closeFlag 并进行写入，保证原子性
	wsConn.Lock()
	defer wsConn.Unlock()

	if atomic.LoadInt32(&wsConn.closeFlag) != 0 {
		return errors.New("connection closed")
	}

	return wsConn.conn.WriteMessage(websocket.BinaryMessage, msg)
}
