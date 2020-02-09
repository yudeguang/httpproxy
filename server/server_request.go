package server

import (
	"fmt"
	"log"
	"net"
	"strings"
)

var is_debug_model bool

type serverRequest struct {
	sessIndex  int64
	remoteIP   string
	dataText   string
	header     *MyHeader
	clientName string
}

func (this *serverRequest) printInfo(args ...interface{}) {
	if is_debug_model {
		log.Println(fmt.Sprintf("[%d@%s]", this.sessIndex, this.remoteIP), fmt.Sprint(args...))
	}
}
func (this *serverRequest) doRequest(conn net.Conn, header *MyHeader, dataText string, sessIndex int64, remoteIP string) interface{} {
	this.header = header
	this.dataText = dataText
	this.sessIndex = sessIndex
	this.remoteIP = remoteIP
	this.clientName = this.getClientName()
	commandId := this.getCommandId()
	if commandId == STATUS_REQUEST_ALIVE {
		info := fmt.Sprintf("收到心跳请求:Client=%s Addr:%s", this.clientName, dataText)
		this.printInfo(info)
		if len(this.clientName) > 0 {
			GetServerManager().AddClientPortAPI(conn, this.clientName, dataText)
		}
		return nil
	} else if commandId == STATUS_REQUEST_PROXY_CONN_OK {
		info := fmt.Sprintf("代理成功消息:RequestId:%d", header.RequestId)
		this.printInfo(info)
		GetServerManager().AddChannelOnProxySuccess(header.RequestId, conn)
		return nil
	}
	return nil
}

func (this *serverRequest) getCommandId() int32 {
	return this.header.CommandId
}
func (this *serverRequest) getClientName() string {
	clientName := string(this.header.ClientName[:])
	return strings.TrimRight(clientName, string([]byte{0x0}))
}
