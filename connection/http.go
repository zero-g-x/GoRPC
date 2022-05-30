package connection

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

//****************server
const(
	connected = "200 Connected to RPC"
	defaultPRCPath = ""
	defaultDebugPath = ""
)

// Server implements http.Handler
func (server *Server)ServeHTTP(w http.ResponseWriter,req *http.Request){
	if req.Method!="CONNECT"{
		w.Header().Set("Content-Type","test/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_,_=io.WriteString(w,"405 must CONNECT\n")
		return 
	}
	conn,_,err:=w.(http.Hijacker).Hijack()
	if err!=nil{
		log.Println("rpc hijaking ",req.RemoteAddr," error: ",err.Error())
		return 
	}
	_,_=io.WriteString(conn,"HTTP/1.0 "+connected+"\n\n")
	server.ServeConn(conn)
}

func (server *Server)HandleHttp(){
	http.Handle(defaultPRCPath,server)
}

//*************client

func NewHttpClient(conn net.Conn,option *Option)(client *Client,err error){
	_,_ =io.WriteString(conn,fmt.Sprintf("CONNECT %s HTTP/1.0\n\n",defaultDebugPath))
	// wait for response:
	resp,err:=http.ReadResponse(bufio.NewReader(conn),&http.Request{Method: "CONNECT"})
	if err==nil&&resp.Status==connected{
		return NewClient(conn,option)
	}
	if err==nil{
		err=errors.New("unexpected http response: "+resp.Status)
	}
	return nil,err
}

// connects to http rpc server
func DialHttp(network,address string,opts ...*Option)(*Client,error){
	return dialTimeOut(NewHttpClient,network,address,opts...)
}

//rpcaddr format: protocol@addr
func Xdial(rpcaddr string ,opts ...*Option)(*Client,error){
	parts:=strings.Split(rpcaddr,"@")
	if len(parts)!=2{
		return nil,fmt.Errorf("client xdial with error format: %s",rpcaddr)
	}
	protocol,addr:=parts[0],parts[1]
	switch protocol{
	case "http":
		return DialHttp("tcp",addr,opts...)
	default:
		return Dial(protocol,addr,opts...)
	}
}

