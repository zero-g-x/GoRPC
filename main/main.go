package main

import (
	gorpc "GoRPC"
	"GoRPC/codec"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

//simple client
func startServer(addr chan string) {
	listen, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error: ", err)
	}
	log.Println("start rpc server on ", listen.Addr())
	addr <- listen.Addr().String()
	gorpc.Accept(listen)
}

func simpleClient() {
	addr := make(chan string)
	go startServer(addr)

	conn, _ := net.Dial("tcp", <-addr)
	defer func() {
		_ = conn.Close()
	}()
	time.Sleep(time.Second)
	//send option:
	_ = json.NewEncoder(conn).Encode(gorpc.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("rpc request, seq=%d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)

	}
}

func clientTest() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)
	client, _ := gorpc.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("this is client rpc request %d ", i)
			var reply string
			if err := client.Call("clientTest", args, &reply); err != nil {
				log.Fatal("client call error ", err)
			}
			log.Println("receive reply: ", reply)
		}(i)
	}
	wg.Wait()
}

type Args struct{ Num1, Num2 int }

type Foo int

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func serviceTest() {
	var foo Foo
	gorpc.Register(&foo)
	addr := make(chan string)
	go startServer(addr)
	client, _ := gorpc.Dial("tcp", <-addr)
	defer func() {
		_ = client.Close()
	}()
	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			ctx,_:=context.WithTimeout(context.Background(),time.Second)
			var reply int
			if err := client.CallWithTimeout(ctx,"Foo.Sum", args, &reply); err != nil {
				log.Println("call Foo.Sum error: ", err)
			}
			log.Println("args: ", args.Num1, ",", args.Num2, ",reply:", reply)
		}(i)
	}
	wg.Wait()
	// args:=&Args{Num1: 22,Num2: 1234}
	// var reply int
	// if err:=client.Call("Foo.Sum",args,&reply);err!=nil{
	// 	log.Println("call Foo.Sum error: ",err)
	// }
	// log.Println("args: ",args.Num1,",",args.Num2,",reply:",reply)
}

func main() {
	serviceTest()
}
