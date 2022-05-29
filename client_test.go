package gorpc

import (
	"context"
	"log"
	"net"
	"testing"
	"time"
)

func TestClientDialTimeout(t *testing.T){
	t.Parallel()
	l,_:=net.Listen("tcp",":0")

	f:=func(conn net.Conn,opt *Option)(client *Client,err error){
		_=conn.Close()
		time.Sleep(time.Second*2)
		return nil,nil
	}
	t.Run("timeout",func(t *testing.T){
		_,err:=dialTimeOut(f,"tcp",l.Addr().String(),&Option{ConnectTimeout: 0})
		if err==nil{
			log.Panic("expected client dial timeout")
		}
	})

	t.Run("no time limits",func(t *testing.T){
		_,err:=dialTimeOut(f,"tcp",l.Addr().String(),&Option{ConnectTimeout: time.Second*100000})
		if err!=nil{
			log.Fatal("client dial without timeout failed",err)
		}
	})

}

type Bar int 

func (b Bar)Timeout(argv int,reply *int) error{
	time.Sleep(time.Second*2)
	return nil
}

func startServerWithBar(addr chan string){
	var b Bar
	_ = Register(&b)
	lis,_:=net.Listen("tcp",":0")
	addr<-lis.Addr().String()
	Accept(lis)
}

func TestClientCallWithTimeout(t *testing.T){
	t.Parallel()
	addrChan:=make(chan string)
	go startServerWithBar(addrChan)
	addr:=<-addrChan
	time.Sleep(time.Second)
	// t.Run("client timeout",func(t *testing.T){
	// 	client,_:=Dial("tcp",addr)
	// 	//log.Println("dialed client: ",client)
	// 	ctx,_:=context.WithTimeout(context.Background(),time.Second)
	// 	var reply int
	// 	err:=client.CallWithTimeout(ctx,"Bar.Timeout",1,&reply)
	// 	if err==nil{
	// 		log.Panic("expected client timeout ",ctx.Err().Error())
	// 	}
	// })

	t.Run("server handle timeout",func(t *testing.T){
		client,_:=Dial("tcp",addr,&Option{HandleTimeout: time.Second,ConnectTimeout: 1*time.Second})
		
		var reply int
		err:=client.CallWithTimeout(context.Background(),"Bar.Timeout",1,&reply)
		// if err==nil{
		// 	log.Panic("expected server timeout ")
		// }
		log.Println(err)
	})
}