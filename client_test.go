package gorpc

import (
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