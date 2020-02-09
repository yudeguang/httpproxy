package server

import (
	"fmt"
	"github.com/yudeguang/stringsx"
	"log"
	"net"
	"runtime/debug"
	"strings"
	"sync/atomic"
)

var plSessionIndexNumber = new(int64)

func recoverSession() {
	if r := recover(); r != nil {
		log.Println("panic", r)
		debug.PrintStack()
	}
}

type serverSession struct {
	conn          net.Conn
	remoteIP      string
	sessIndex     int64
	needAutoClose bool
}

func (this *serverSession) printInfo(args ...interface{}) {
	if !(strings.Contains(stringsx.JoinInterface(" ", args), "收到") ||
		strings.Contains(stringsx.JoinInterface(" ", args), "断开")) {
		log.Println(fmt.Sprintf("[%d@%s]", this.sessIndex, this.remoteIP), fmt.Sprint(args...))
	}

}
func (this *serverSession) Run() {
	var err error
	defer func() {
		if this.needAutoClose && this.conn != nil {
			defer this.conn.Close()
		}
	}()
	defer recoverSession()
	this.needAutoClose = true
	this.sessIndex = atomic.AddInt64(plSessionIndexNumber, 1)
	this.remoteIP, _, err = net.SplitHostPort(this.conn.RemoteAddr().String())
	if err != nil {
		panic(err)
	}
	this.printInfo("收到链接...")
	defer this.printInfo("断开连接")
	this.loopWork()
}

func (this *serverSession) loopWork() {
	requester := &serverRequest{}
	for {
		header, dataText, err := SocketReadPackage(this.conn, 10)
		if err != nil {
			this.printInfo("读取数据错误:", err)
			return
		}
		//调用对象处理请求和返回结果
		resObject := requester.doRequest(this.conn, header, dataText, this.sessIndex, this.remoteIP)
		if header.CommandId == STATUS_REQUEST_PROXY_CONN_OK {
			//如果是回复代理成功,那么不回复了,并且跳出，由代理程序工作
			this.needAutoClose = false
			break
		}
		this.replyClient(header, resObject)
	}
}

//回复Client数据
func (this *serverSession) replyClient(header *MyHeader, resObject interface{}) error {
	header.CommandId = STATUS_OK
	var replyText = ""
	if resObject != nil {
		if err, ok := resObject.(error); ok {
			header.CommandId = STATUS_ERROR
			replyText = err.Error()
		} else if text, ok := resObject.(string); ok {
			replyText = text
		} else {
			replyText = fmt.Sprint(resObject)
		}
	}
	return SocketWritePackage(this.conn, 10, header, replyText)
}
