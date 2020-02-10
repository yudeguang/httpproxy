package server

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

//当前正确使用的客户端
var curInUseClientNames sync.Map

//通过http代码并用GET方式获取数据
func ProxyGet(URL string) (HTML string, ClientName string, err error) {
	//自己定义的httpClient,用这个client调用请求
	var ProxyHttpClient = &http.Client{
		Transport: &http.Transport{
			Dial:            proxyDialFunction,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	u, err := url.Parse(URL)
	if err != nil {
		return
	}
	//记得清空
	defer curInUseClientNames.Store(u.Host, "")
	resp, err := ProxyHttpClient.Get(URL) //代理方法，自动从多个客户端中找一个来访问
	if err != nil {
		HTML = err.Error()
	} else {
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			HTML = err.Error()
		} else {
			HTML = string(data)
		}
	}
	v, _ := curInUseClientNames.Load(u.Host)
	ClientName = fmt.Sprint(v)
	//log.Println(curInUseClientNames.Load(u.Host))
	return
}

//自己实现的代理函数,不直接连接，从代理服务器中获取到连接
func proxyDialFunction(network, addr string) (net.Conn, error) {
	log.Println("proxyDialFunction连接地址:", network, addr) //能输出到这个的话说明是进入了这个函数
	//对这个地址的请求我们不直接连接，用活跃的Client给我们代理处理一下
	clientName, conn, err := GetServerManager().GetPorxyTCPConnect(addr)
	//不允许访问太快,尽量同时只让一个客户端运行
	timeout := time.After(time.Second * 5)
	for {
		curclientName, exist := curInUseClientNames.Load(addr)
		//不存在，或者是空的，说明目前没人在用
		if !exist {
			//log.Println("退出", "!exist")
			break
		}
		if fmt.Sprint(curclientName) == "" {
			//log.Println("退出", curclientName)
			break
		}
		time.Sleep(time.Millisecond * 100)
		//log.Println("休眠100")
		//超时也退出，最多等5秒
		select {
		case <-timeout:
			curInUseClientNames.Store(addr, "")
			break
		}
	}
	curInUseClientNames.Store(addr, clientName)
	if clientName == "" {
		log.Println("没有找到合适的Client代理客户端:", addr)
	} else {
		if err == nil {
			log.Println(network + ":查找到Client=" + clientName + " 代理:" + addr + " Conn连接成功...")
		} else {
			log.Println(network + ":查找到Client=" + clientName + " 代理:" + addr + " 失败:" + err.Error())
		}
	}
	return conn, err
}
