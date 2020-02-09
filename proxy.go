package httpproxy

import (
	"./client"
	"./server"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

var fullClientName string

//启动客户端
//clientName ==> example_v1     客户端名称
//clientAddr ==> 127.0.0.1:8085 本地客户端地址
//serverAddr ==> 8.8.8.8:8888   远程服务器地址
func Start_client(clientName, clientAddr, serverAddr string) {
	go client.Proxy_client_start(clientName, clientAddr, serverAddr)
}

//启动服务器
//client_version == v1 对应clientName，只有clientName包含client_version关键字时，相应客户端才生效
//serverAddrPort == 8888 服务器端口
func Start_server(client_version, serverAddrPort string) {
	go server.GetServerManager().Server_start(client_version, serverAddrPort)
}

//自己定义的httpClient,用这个client调用请求
var ProxyHttpClient = &http.Client{
	Transport: &http.Transport{
		Dial:            proxyDialFunction,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

/*
封装好的,通过常理方式访问
*/
//通过代理访问数据
func ProxyGet(URL string) (HTML string, ClientName string, err error) {
	ClientName = fullClientName
	fullClientName = ""
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
	return
}

//自己实现的代理函数,不直接连接，从代理服务器中获取到连接
func proxyDialFunction(network, addr string) (net.Conn, error) {
	log.Println("proxyDialFunction连接地址:", network, addr) //能输出到这个的话说明是进入了这个函数
	//对这个地址的请求我们不直接连接，用活跃的Client给我们代理处理一下
	clientName, conn, err := server.GetServerManager().GetPorxyTCPConnect(addr)
	fullClientName = clientName
	if clientName == "" {
		log.Println("无Client代理匹配:", addr)
	} else {
		if err == nil {
			log.Println("查找到Client=" + clientName + " 代理:" + addr + " Conn连接成功...")
		} else {
			log.Println("查找到Client=" + clientName + " 代理:" + addr + " 失败:" + err.Error())
		}
	}
	return conn, err
}
