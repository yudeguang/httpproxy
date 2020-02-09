package httpproxy

import (
	"github.com/yudeguang/httpproxy/client"
	"github.com/yudeguang/httpproxy/server"
)

//启动客户端
//clientName ==> example_v1     客户端名称
//clientAddr ==> 127.0.0.1:8085 本地客户端地址
//serverAddr ==> 8.8.8.8:8888   远程服务器地址
func Client_start(clientName, clientAddr, serverAddr string) {
	go client.Proxy_client_start(clientName, clientAddr, serverAddr)
}

//启动服务器
//client_version == v1 对应clientName，只有clientName包含client_version关键字时，相应客户端才生效
//serverAddrPort == 8888 服务器端口
func Server_start(client_version, serverAddrPort string) {
	go server.GetServerManager().Server_start(client_version, serverAddrPort)
}

//注意，ClientName并不是绝对准确
func Server_proxy_get(URL string) (HTML string, ClientName string, err error) {
	return server.ProxyGet(URL)
}
