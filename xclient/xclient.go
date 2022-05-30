package xclient

import (
	"GoRPC/connection"
	"context"
	"io"
	"reflect"
	"sync"

	"golang.org/x/tools/go/analysis/passes/nilfunc"
)

type XClient struct{
	d Discovery
	mode SelectMode
	opt *connection.Option
	mu sync.Mutex
	clients map[string]*connection.Client
}

var _ io.Closer=(*XClient)(nil)
func (xc *XClient)Close()error{
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key,client:=range xc.clients{
		_=client.Close()
		delete(xc.clients,key)
	}
	return nil
}

func NewXClient(d Discovery,mode SelectMode,opt *connection.Option)*XClient{
	return &XClient{
		d: d,
		mode: mode,
		opt: opt,
		clients: make(map[string]*connection.Client),
	}
}

func (xc *XClient)dial(addr string)(*connection.Client,error){
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client,ok:=xc.clients[addr]
	if !ok||!client.Available(){
		_=client.Close()
		delete(xc.clients,addr)
		client=nil
	}
	if client==nil{
		// no client match,try to dial
		var err error
		client,err=connection.Xdial(addr,xc.opt)
		if err!=nil{
			return nil,err
		}
		xc.clients[addr]=client
	}
	return client,nil
}

func (xc *XClient)Call(ctx context.Context,serviceMethod string,args,reply interface{})error{
	addr,err:=xc.d.Get(xc.mode)
	if err!=nil{
		return err
	}
	return xc.call(addr,ctx,serviceMethod,args,reply)
}

func (xc *XClient)call(addr string ,ctx context.Context,serviceMethod string,args,reply interface{})error{
	client,err:=xc.dial(addr)
	if err!=nil{
		return err
	}
	return client.CallWithTimeout(ctx,serviceMethod,args,reply)
}


// spread request to all servers
func (xc *XClient)Broadcast(ctx context.Context,serviceMethod string,argv,reply interface{})error{
	servers,err:=xc.d.GetAll()
	if err!=nil{
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var e error

	done:=reply==nil
	ctx,cancel:=context.WithCancel(ctx)

	for _,addr:=range servers{
		wg.Add(1)
		go func(addr string){
			defer wg.Done()
			var clonedReply interface{}
			if reply!=nil{
				clonedReply=reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err:=xc.call(addr,ctx,serviceMethod,argv,clonedReply)
			mu.Lock()
			if err!=nil&&e==nil{
				e=err
				cancel()
			}
			if err==nil&&!done{
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				done=true
			}
			mu.Unlock()
		}(addr)
	}	
	wg.Wait()
	return e
}