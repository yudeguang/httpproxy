package server

import (
	"fmt"
	"github.com/yudeguang/haserr"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//定义使用的端口，默认8888
var ServerListenPort = 8888
var Proxy_client_version string

var plProxyAddrIndexNumber = new(int64)
var pServerManager *serverManager = nil

func GetServerManager() *serverManager {
	if pServerManager == nil {
		pServerManager = &serverManager{}
		pServerManager.mapClient = make(map[string]*tagPorxyAddrItem)
		pServerManager.mapChannel = make(map[int64]chan net.Conn)
	}
	return pServerManager
}

//代理地址结构体
type tagPorxyAddrItem struct {
	conn          net.Conn //这个client的活跃连接
	clientName    string
	proxyAddr     string //代理的地址
	lastHeartTime int64  //上次的心跳时间
	lastUseTime   int64  //最近一次用这个client的这个代理的时间,下一次可能就不用这个了，选时间隔的最远的
}

//server管理对象
type serverManager struct {
	mapLocker sync.Mutex
	//用于存储代理地址与Client的关系,一个地址可以被多个Client代理
	//key为clientName+"##"+proxyAddr,所以定义的clientName中不能有两个##
	mapClient map[string]*tagPorxyAddrItem
	//mapChannel的关系
	chanLocker sync.Mutex
	mapChannel map[int64]chan net.Conn
}

func (this *serverManager) Server_start(client_version, serverAddrPort string) {
	log.Println("Server切换到目录:" + ChangeToBinDir())
	var err error
	ServerListenPort, err = strconv.Atoi(serverAddrPort)
	haserr.Panic(err)
	Proxy_client_version = client_version
	this.listenAndAccept()
}

func (this *serverManager) Stop() {

}

//启动socket服务
func (this *serverManager) listenAndAccept() {
	var tempDelay time.Duration // how long to sleep on accept failure
	defaultAddr := fmt.Sprintf(":%d", ServerListenPort)
	log.Println("启动到地址 ", defaultAddr, "上的Socket服务...")
	ln, err := net.Listen("tcp", defaultAddr)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			panic(err)
		}
		session := &serverSession{conn: conn}
		go session.Run()
	}
}

//添加client上报信息
func (this *serverManager) AddClientPortAPI(aliveConn net.Conn, clientName string, supportAddr string) {
	var lstAddr = strings.Split(supportAddr, ",")
	this.mapLocker.Lock()
	defer this.mapLocker.Unlock()
	var nowTime = time.Now().Unix()
	//找到这个代理地址
	for _, addr := range lstAddr {
		addr = strings.TrimSpace(addr)
		if len(addr) == 0 {
			continue
		}
		key := clientName + "##" + addr
		item, ok := this.mapClient[key]
		if ok && item != nil {
			//找到了,更新下时间就可以
			item.lastHeartTime = nowTime
			item.conn = aliveConn
		} else {
			//没有找到,
			newItem := &tagPorxyAddrItem{
				clientName:    clientName,
				proxyAddr:     addr,
				lastHeartTime: nowTime,
				conn:          aliveConn,
				lastUseTime:   0,
			}
			this.mapClient[key] = newItem
		}
	}
}

//获得一个代理的net.Conn连接对象
func (this *serverManager) GetPorxyTCPConnect(addr string) (clientName string, conn net.Conn, err error) {
	//并且使用时间距离现在最远的
	var nowTime = time.Now().Unix()
	var findItem *tagPorxyAddrItem = nil
	this.mapLocker.Lock()
	for _, proxyItem := range this.mapClient {
		//先确定用户名，或者说是客户端版本
		if strings.Contains(proxyItem.clientName, Proxy_client_version) {
			//log.Println(proxyItem.clientName, Proxy_client_version)
			//先判断地址是否一致如127.0.0.1:8087
			if proxyItem.proxyAddr != addr {
				continue
			}
			//找到一个lastHeartTime在7秒内的，因为3秒一个心跳,允许有一次心跳没到达
			if nowTime-proxyItem.lastHeartTime > 7 {
				continue
			}
			//找到一个就先用上
			if findItem == nil {
				findItem = proxyItem
				//找到最长时间没使用的，再换掉
			} else {
				if proxyItem.lastUseTime < findItem.lastUseTime {
					findItem = proxyItem
				}
			}
		}
	}
	this.mapLocker.Unlock()
	if findItem == nil {
		err = fmt.Errorf("not found proxy client for:%s", addr)
		return
	}
	clientName = findItem.clientName
	findItem.lastUseTime = time.Now().Unix()
	conn, err = this.sendClientPorxyRequest(findItem.conn, addr)
	return
}

//向这个长连接发一个请求，需要代理某个地址
func (this *serverManager) sendClientPorxyRequest(conn net.Conn, addr string) (newconn net.Conn, newErr error) {
	newconn = nil
	header := &MyHeader{}
	header.CommandId = STATUS_REQUEST_PROXY_ADDR
	header.HeadFlag = HEADER_FLAG
	reqId := atomic.AddInt64(plProxyAddrIndexNumber, 1)
	header.RequestId = reqId
	ch := make(chan net.Conn, 1)
	this.addChannelOnBegin(reqId, ch)
	newErr = SocketWritePackage(conn, 5, header, addr)
	if newErr != nil {
		this.delChannelOnEnd(reqId)
		return
	}
	select {
	case newconn = <-ch: //拿到连接
		log.Println("收到代理地址:", addr, "的反连接...")
		break
	case <-time.After(5 * time.Second): //超时5s
		newErr = fmt.Errorf("等待代理地址:%s 的反连接超时", addr)
	}
	this.delChannelOnEnd(reqId)
	if newconn == nil && newErr == nil {
		newErr = fmt.Errorf("反连接对象为nil")
	}
	if newErr != nil && newconn != nil {
		newconn.Close()
		newconn = nil
	}
	//log.Println("mapChannel长度:", len(this.mapChannel))
	return
}
func (this *serverManager) AddChannelOnProxySuccess(reqId int64, conn net.Conn) {
	this.chanLocker.Lock()
	ch := this.mapChannel[reqId]
	this.chanLocker.Unlock()
	timeout := time.NewTimer(time.Microsecond * 50)
	if ch != nil {
		select {
		case ch <- conn:
			break
		case <-timeout.C:
			//fmt.Println("************************111")
			conn.Close()
		}
	} else {
		//fmt.Println("222222222222222222111")
		conn.Close()
	}
}

func (this *serverManager) addChannelOnBegin(reqId int64, ch chan net.Conn) {
	this.chanLocker.Lock()
	defer this.chanLocker.Unlock()
	oldch := this.mapChannel[reqId]
	if oldch != nil {
		close(oldch)
	}
	this.mapChannel[reqId] = ch
}

func (this *serverManager) delChannelOnEnd(reqId int64) {
	this.chanLocker.Lock()
	defer this.chanLocker.Unlock()
	ch := this.mapChannel[reqId]
	if ch != nil {
		close(ch)
	}
	delete(this.mapChannel, reqId)
}
