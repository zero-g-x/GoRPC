package main

import (
	gorpc "GoRPC"
	"GoRPC/codec"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

//simple client
func startServer(addr chan string){
	listen,err := net.Listen("tcp",":8080")
	if err!=nil{
		log.Fatal("network error: ",err)
	}
	log.Println("start rpc server on ",listen.Addr())
	addr <- listen.Addr().String()
	gorpc.Accept(listen)
}

func main(){
	addr := make(chan string)
	go startServer(addr)

	conn,_ := net.Dial("tcp",<-addr)
	defer func(){
		_ = conn.Close()
	}()
	time.Sleep(time.Second)
	//send option:
	_ = json.NewEncoder(conn).Encode(gorpc.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i:=0;i<5;i++{
		h:=&codec.Header{
			ServiceMethod: "",
			Seq: uint64(i),
		}
		_=cc.Write(h,fmt.Sprintf("rpc request, seq=%d",h.Seq))
		_=cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:",reply)
		
	}

}