package client

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

//配置参数,subnet_proxy_client.ini中的内容
type tagConfigItem struct {
	serverAddr string //中心服务IP地址
	proxyAddrs string //支持的代理地址
	clientName string //该client的名字
}

var configer *tagConfigItem = nil

//保持存活的长连接
var keepAliveConn net.Conn

//启动客户端
//clientName ==> example_v1     客户端名称
//clientAddr ==> 127.0.0.1:8085 本地客户端地址
//serverAddr ==> 127.0.0.1:8888 远程服务器地址
func Proxy_client_start(clientName, clientAddr, serverAddr string) {
	var err error = nil
	log.SetFlags(log.LstdFlags | log.LstdFlags)
	log.Println("程序开始...")
	log.Println("切换到目录:" + ChangeToBinDir())
	configer, err = checkConfig(clientName, clientAddr, serverAddr)
	if err != nil {
		log.Println("读取配置错误:", err)
		os.Exit(-1)
	}
	//一直一个死循
	for {
		err := connectKeepAliveSocket()
		if err != nil {
			log.Println("连接服务器失败:", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Println("连接服务器成功...")
		var wg sync.WaitGroup
		//环等待两个线程退出
		wg.Add(2)
		go keepAliveThread(&wg)
		go recvRequestThread(&wg)
		wg.Wait()
		time.Sleep(2 * time.Second)
	}
}

//读取配置文件
func checkConfig(clientName, clientAddrs, serverAddr string) (*tagConfigItem, error) {
	result := &tagConfigItem{}
	result.clientName = clientName
	result.serverAddr = serverAddr
	result.proxyAddrs = clientAddrs //  proxyaddr

	if !strings.Contains(result.serverAddr, ":") {
		return nil, fmt.Errorf("serverAddr配置参数错误，请参考:127.0.0.1:8888")
	}
	if !strings.Contains(result.proxyAddrs, ":") {
		return nil, fmt.Errorf("clientAddr配置参数错误，请参考:127.0.0.1:8085")
	}
	if result.clientName == "" {
		return nil, fmt.Errorf("clientname配置不能为空")
	}
	result.proxyAddrs = strings.Replace(result.proxyAddrs, ";", ",", -1)
	log.Println("服务器地址:", result.serverAddr)
	log.Println("支持能力:", result.proxyAddrs)
	log.Println("ClientName:", result.clientName)
	return result, nil
}

//建立长连接
func connectKeepAliveSocket() error {
	var err error
	if keepAliveConn != nil {
		keepAliveConn.Close()
		keepAliveConn = nil
	}
	keepAliveConn, err = net.DialTimeout("tcp", configer.serverAddr, 5*time.Second)
	if err != nil {
		return err
	}
	keepAliveConn.(*net.TCPConn).SetLinger(2)
	return nil
}

//心跳线程,用于保持对服务器的连接,每3秒发送一次,只发不收
func keepAliveThread(pwg *sync.WaitGroup) {
	defer func() {
		keepAliveConn.Close()
		pwg.Done()
	}()
	bodyBytes := []byte(configer.proxyAddrs)
	header := newMyHeaderWithClientName()
	header.CommandId = STATUS_REQUEST_ALIVE
	header.DataLen = int32(len(bodyBytes))
	header.HeadFlag = HEADER_FLAG
	headBytes := ConvertHeaderToBytes(header)
	allSendBytes := append(headBytes, bodyBytes...)
	for {
		keepAliveConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		err := SocketWriteAll(keepAliveConn, allSendBytes)
		if err != nil {
			log.Println("发送心跳数据失败,关闭连接...")
			time.Sleep(time.Second)
			break
		}
		time.Sleep(3 * time.Second)
	}
}

//接收请求线程
func recvRequestThread(pwg *sync.WaitGroup) {
	defer func() {
		keepAliveConn.Close()
		pwg.Done()
	}()
	for {
		header, dataText, err := SocketReadPackage(keepAliveConn, 10)
		if err != nil {
			log.Println("读取数据错误:", err)
			return
		}
		if header.CommandId == STATUS_OK {
			//log.Println("收到存活响应...")
		} else if header.CommandId == STATUS_REQUEST_PROXY_ADDR {
			//创建一个线程来新建一个连接并打通端口连接
			go portProxyThread(dataText, header.RequestId)
		} else {
			log.Println("未知的回复代码:", header.CommandId)
		}
		if len(dataText) > 0 {
			log.Println("回复内容为:", dataText)
		}
	}
}

//获得一个头，这里因为都要带clientname所以封装一个函数来用
func newMyHeaderWithClientName() *MyHeader {
	var header MyHeader
	copy(header.ClientName[:], []byte(configer.clientName))
	return &header
}

//执行代理线程
func portProxyThread(proxyAddr string, requestId int64) {
	log.Println(fmt.Sprintf("[RequestId=%d]收到请求代理地址:%s...", requestId, proxyAddr))
	//与服务端建立一个新的socket,用这个socket做代理
	remoteConn, err := net.DialTimeout("tcp", configer.serverAddr, 5*time.Second)
	if err != nil {
		log.Println(err)
		return
	}
	defer remoteConn.Close()
	remoteConn.(*net.TCPConn).SetLinger(0)
	//先准备好头,连接成功了就要向服务端反馈
	header := newMyHeaderWithClientName()
	header.CommandId = STATUS_REQUEST_PROXY_CONN_ERROR
	header.DataLen = 0
	header.HeadFlag = HEADER_FLAG
	header.RequestId = requestId
	header.Res1 = 0
	//再连接本地端口
	localConn, err := net.DialTimeout("tcp", proxyAddr, 3*time.Second)
	if err != nil {
		log.Println(fmt.Sprintf("[RequestId=%d]连接目标:%s 失败:%v", requestId, proxyAddr, err))
		header.CommandId = STATUS_REQUEST_PROXY_CONN_ERROR
		SocketWriteAll(remoteConn, ConvertHeaderToBytes(header))
		time.Sleep(1 * time.Second)
		return
	}
	defer localConn.Close()
	localConn.(*net.TCPConn).SetLinger(0)
	log.Println(fmt.Sprintf("[RequestId=%d]连接目标地址:%s 成功...", requestId, proxyAddr))
	//两个接口都连接成功,好了。回复可以
	header.CommandId = STATUS_REQUEST_PROXY_CONN_OK
	SocketWriteAll(remoteConn, ConvertHeaderToBytes(header))
	log.Println(fmt.Sprintf("[RequestId=%d]代理数据中...", requestId))
	//交换两个socket
	pwg := &sync.WaitGroup{}
	pwg.Add(2)
	go swapSocket(localConn, remoteConn, pwg)
	go swapSocket(remoteConn, localConn, pwg)
	pwg.Wait()
	log.Println(fmt.Sprintf("[RequestId=%d]<<<代理连接结束>>>", requestId))
}

func swapSocket(dst net.Conn, src net.Conn, pwg *sync.WaitGroup) {
	defer func() {
		dst.Close()
		src.Close()
		pwg.Done()
	}()
	io.Copy(dst, src)
}
