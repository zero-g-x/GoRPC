package gorpc

import (
	"GoRPC/codec"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

func (call *Call) done() {
	call.Done <- call
}

// a Client can handle mutiple calls
type Client struct {
	codec        codec.Codec
	option       *Option
	sendmu       sync.Mutex
	header       codec.Header
	mutex        sync.Mutex
	seq          uint64
	pendingCalls map[uint64]*Call
	closing      bool //client call close
	shutdown     bool //server shut down
}

type clientResult struct{
	client *Client
	err error
}

type newClientFunc func(conn net.Conn,opt *Option)(*Client,error)

var ErrShutDone = errors.New("Connection shut down")

var _ io.Closer = (*Client)(nil) //check if Client implement io.Closer

func (client *Client) Close() error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.closing {
		return ErrShutDone
	}
	client.closing = true
	return client.codec.Close()
}

func (client *Client) Available() (res bool) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	res = !(client.shutdown || client.closing)
	return
}

//register call and increse seq
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.shutdown || client.closing {
		return 0, ErrShutDone
	}
	call.Seq = client.seq
	client.pendingCalls[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	call := client.pendingCalls[seq]
	delete(client.pendingCalls, seq)
	return call
}

func (client *Client) terminate(err error) {
	client.sendmu.Lock()
	defer client.sendmu.Unlock()
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.shutdown = true
	for _, call := range client.pendingCalls {
		call.Error = err
		call.done()
	}
}

func (client *Client) receive() {
	var err error
	//fmt.Println("in client receive")
	for err == nil {
		var header codec.Header
		if err = client.codec.ReadHeader(&header); err != nil {
			log.Println("client receive header error", err)
			break
		}
		//fmt.Println("reove call:", header.Seq)
		call := client.removeCall(header.Seq)
		// if call==nil{//call not exists
		// 	err=client.codec.ReadBody(nil)
		// }else if header.Error!=""{//call exists but server error
		// 	call.Error=fmt.Errorf(header.Error)
		// 	err=client.codec.ReadBody(nil)
		// 	call.done()//
		// }else{// read body
		// 	err = client.codec.ReadBody(call.Reply)
		// 	if err!=nil{
		// 		call.Error=errors.New("client read body error: "+err.Error())
		// 	}
		// 	call.done()
		// }
		//fmt.Println("client receive.", client.seq)
		switch {
		case call == nil:
			err = client.codec.ReadBody(nil)
		case header.Error != "":
			call.Error = fmt.Errorf(header.Error)
			err = client.codec.ReadBody(nil)
			call.done()
		default:
			err = client.codec.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("client receive read body err: " + err.Error())
			}
			call.done()
		}
	}
	client.terminate(err)
}

func NewClient(conn net.Conn, option *Option) (client *Client, err error) {
	fmt.Println("call new client")
	newCodecFunc := codec.NewCodecFuncMap[option.CodecType]
	if newCodecFunc == nil {
		err = fmt.Errorf("NewClient invalid codec type %s", option.CodecType)
		log.Println(err)
		return nil, err
	}
	//send option to server
	if err = json.NewEncoder(conn).Encode(option); err != nil {
		log.Println("rpc client encode option error: ", err)
		return nil, err
	}
	client = &Client{
		seq:          1,
		codec:        newCodecFunc(conn),
		option:       option,
		pendingCalls: make(map[uint64]*Call),
	}
	//fmt.Println("go client receive")
	go client.receive()
	return
}

func parseOption(opts ...*Option) (*Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("client parse options more than 1")
	}
	option := opts[0]
	if option.CodecType == "" {
		option.CodecType = DefaultOption.CodecType
	}
	option.MagicNumber = DefaultOption.MagicNumber
	return option, nil
}

//connects to server
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	// opt, err := parseOption(opts...)
	// if err != nil {
	// 	return nil, err
	// }
	// conn, err := net.Dial(network, address)
	// client, err = NewClient(conn, opt)

	// if client == nil {
	// 	_ = conn.Close()
	// }
	// fmt.Println("dial")
	return dialTimeOut(NewClient,network,address,opts...)
}

func dialTimeOut(f newClientFunc,network,address string,opts ...*Option)(*Client,error){
	
	opt,err:=parseOption(opts...)
	if err!=nil{
		log.Println("client dialTimeout err, parse option failed: ",err)
		return nil,err
	}
	var conn net.Conn
	if opt.ConnectTimeout<=0{
		conn,err=net.Dial(network,address)
	}else{
		conn,err=net.DialTimeout(network,address,opt.ConnectTimeout)
	}
	defer func(){
		if err!=nil{
			_=conn.Close()//close connection if error
		}
	}()
	
	ch:=make(chan clientResult)
	go func(){
		client,err:=f(conn,opt)
		ch<-clientResult{
			client: client,
			err: err,
		}
	}()
	
	select{
	case <-time.After(opt.ConnectTimeout):
		log.Println("client connect dial timeout within ",opt.ConnectTimeout)
		return nil,fmt.Errorf("client connect dial timeout within %s ",opt.ConnectTimeout)
	case result:=<-ch:
		return result.client,result.err
	}

}

func (client *Client) send(call *Call) {
	client.sendmu.Lock()
	defer client.sendmu.Unlock()
	//register call
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	//request header:
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	err = client.codec.Write(&client.header, call.Args)
	if err != nil {
		call = client.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
	//fmt.Println("client send call ", client.header.Seq, ",", client.seq)
}

//invoke asynchronously
func (client *Client) Go(serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("client Go done channel unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	//log.Println(" client send call")
	client.send(call)
	//
	return call
}

func (client *Client) Call(serviceMethod string, args interface{}, reply interface{}) error {

	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error	
	//block until call is done
	//fmt.Println("args:",call.Args,",reply:",call.Reply)
	// call:=client.Go(serviceMethod,args,reply,make(chan *Call,1))
	// select{
	// case <-ctx.Done():
	// 	client.removeCall(call.Seq)
	// 	return errors.New("client call timeout "+ctx.Err().Error())
	// case call=<-call.Done:
	// 	return call.Error
	// }
}

func (client *Client) CallWithTimeout(ctx context.Context,serviceMethod string, args interface{}, reply interface{}) error {

	//call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	//block until call is done
	//fmt.Println("args:",call.Args,",reply:",call.Reply)
	call:=client.Go(serviceMethod,args,reply,make(chan *Call,1))
	select{
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("client call timeout "+ctx.Err().Error())
	case call=<-call.Done:
		return call.Error
	}
}