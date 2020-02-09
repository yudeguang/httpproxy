package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unsafe"
)

//切换到当前EXE目录
func ChangeToBinDir() string {
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	dir, _ := filepath.Split(exePath)
	os.Chdir(dir)
	return dir
}

//协议头结构定义
type MyHeader struct {
	HeadFlag   int32    //协议标记头
	CommandId  int32    //请求或动作
	DataLen    int32    //数据长度,不包含头长度
	RequestId  int64    //请求标记
	Res1       int32    //保留字段
	ClientName [32]byte //client名字
}

var HEADER_SIZE = 56                //MyHeader头长度字节
var HEADER_FLAG = int32(0x18120913) //协议标记随便定义一个就可以
var MAX_PACKET_SIZE = int32(1024)   //最大允许的交互packet大小
//状态定义
var STATUS_OK = int32(0)                         //状态OK
var STATUS_ERROR = int32(-1)                     //错误
var STATUS_REQUEST_ALIVE = int32(1)              //上报存活信息
var STATUS_REQUEST_PROXY_ADDR = int32(2)         //请求代理地址
var STATUS_REQUEST_PROXY_CONN_OK = int32(100)    //连接代理地址,可以开始代理了
var STATUS_REQUEST_PROXY_CONN_ERROR = int32(101) //无法连接代理地址
//拆分配置到map
func SplitStringToMap(text string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if npos := strings.Index(line, "="); npos > 0 {
			key := strings.ToLower(line[:npos])
			val := strings.TrimSpace(line[npos+1:])
			m[key] = val
		}
	}
	return m
}

//socket辅助函数
func SocketReadAtLeast(conn net.Conn, nsize int) ([]byte, error) {
	if nsize < 0 || nsize > int(MAX_PACKET_SIZE) {
		return []byte{}, fmt.Errorf("socketReadAtLeast Size Error:%d", nsize)
	}
	buffer := make([]byte, nsize)
	var completeSize = 0
	for completeSize < nsize {
		n, err := conn.Read(buffer[completeSize:])
		if err != nil {
			return buffer, err
		}
		completeSize += n
	}
	return buffer, nil
}

//socket发送数据
func SocketWriteAll(conn net.Conn, data []byte) error {
	var completeSize = 0
	nsize := len(data)
	for completeSize < nsize {
		n, err := conn.Write(data[completeSize:])
		if err != nil {
			return err
		}
		completeSize += n
	}
	return nil
}

//socket读取协议包
func SocketReadPackage(conn net.Conn, timeout int) (*MyHeader, string, error) {
	err := conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
	if err != nil {
		return nil, "", err
	}
	data, err := SocketReadAtLeast(conn, HEADER_SIZE)
	if err != nil {
		return nil, "", err
	}
	var header MyHeader
	header = *(*MyHeader)(unsafe.Pointer(&data[0]))
	if header.HeadFlag != HEADER_FLAG {
		return nil, "", fmt.Errorf("协议头标记错误:0x%.08X", header.HeadFlag)
	}
	if header.DataLen < 0 || header.DataLen > MAX_PACKET_SIZE {
		return nil, "", fmt.Errorf("协议内容长度超出限制:%d", header.DataLen)
	}
	data, err = SocketReadAtLeast(conn, int(header.DataLen))
	if err != nil {
		return nil, "", err
	}
	return &header, string(data), nil
}

//socket发送协议包
func SocketWritePackage(conn net.Conn, timeout int, header *MyHeader, dataText string) error {
	conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(timeout)))
	header.HeadFlag = HEADER_FLAG
	var dataBody = []byte(dataText)
	header.DataLen = int32(len(dataBody))
	var dataHead = ConvertHeaderToBytes(header)
	return SocketWriteAll(conn, append(dataHead, dataBody...))
}

//将结构体转化byte
func ConvertHeaderToBytes(header *MyHeader) []byte {
	var x reflect.SliceHeader
	x.Len = HEADER_SIZE
	x.Cap = HEADER_SIZE
	x.Data = uintptr(unsafe.Pointer(header))
	var btHead = make([]byte, HEADER_SIZE)
	copy(btHead, *(*[]byte)(unsafe.Pointer(&x)))
	return btHead
}
