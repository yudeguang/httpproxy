package server

import (
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
)

//自己定义的httpClient,用这个client调用请求
var ProxyHttpClient = &http.Client{
	Transport: &http.Transport{
		Dial:            proxyDialFunction,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

/*
8888 一般做为SERVER服务端口
bmw_etk 8083
bmw_partlink24 8084
partslink24(audi_vw,porsche) 8085
audi_vw_etka 8086
land_rover_jaguar 8087
*/
//通过代理访问数据
func proxyGet(URL string) (HTML string, err error) {
	resp, err := ProxyHttpClient.Get(URL) //代理方法
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
	clientName, conn, err := GetServerManager().GetPorxyTCPConnect(addr)
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
