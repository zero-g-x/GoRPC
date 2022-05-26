package gorpc

import (
	"GoRPC/codec"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x76543

// code format:json
type Option struct{
	MagicNumber int //marks it's a rpc request
	CodecType codec.Type // format of header and body
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType: codec.GobType,
}

type Server struct{}

func NewServer() *Server{
	return &Server{}
}

var DefaultServer=NewServer()

//accept connections on the listener and serves requests
func (server *Server)Accept(lis net.Listener){
	for{
		conn,err := lis.Accept()
		if err!=nil{
			log.Println("rpc server accept error: ",err)
			return 
		}
		go server.ServeConn(conn)
	}
}
func Accept(lis net.Listener){
	DefaultServer.Accept(lis)
}

func (s *Server)ServeConn(conn io.ReadWriteCloser){
	defer func(){
		_ = conn.Close()
	}()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt);err!=nil{
		log.Println("rpc server: decode option error: ",err)
		return 
	}
	if opt.MagicNumber!=MagicNumber{
		log.Println("rpc server: invalid magic number %x",opt.MagicNumber)
		return 
	}
	newCoderc := codec.NewCodecFuncMap[opt.CodecType]
	if newCoderc==nil{
		log.Println("rpc server: invalid codec type %s",opt.CodecType)
		return
	}
	cc := newCoderc(conn)
	s.serveCodec(cc)
}

func (server *Server)serveCodec(cc codec.Codec){
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for{
		req,err := server.readRequest(cc)
		if err!=nil{
			if req==nil{
				break//close connection
			}
			req.h.Error = err.Error()
			msg := "invalid Request"
			server.sendResponse(cc,req.h,msg,sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc,req,sending,wg)
	}
	wg.Wait()
	_ = cc.Close()
}

//store unformation of a call
type request struct{
	h *codec.Header
	argv,replyv reflect.Value
}

func (server *Server) readRequestHeader(cc codec.Codec)(*codec.Header,error){
	var h codec.Header
	if err:=cc.ReadHeader(&h);err!=nil{
		if err!=io.EOF&&err!=io.ErrUnexpectedEOF{
			log.Println("rpc server:read request header error: ",err)
		}
		return nil,err
	}
	return &h,nil
}

func (server *Server) readRequest(cc codec.Codec)(*request,error){
	h,err := server.readRequestHeader(cc)
	if err!=nil{
		return nil,err
	}
	req:=&request{
		h: h,
	}
	req.argv=reflect.New(reflect.TypeOf(""))
	if err=cc.ReadBody(req.argv.Interface());err!=nil{
		log.Println("rpc server: read argv error: ",err)
	}
	return req,nil
}

func (server *Server)sendResponse(cc codec.Codec,h *codec.Header,body interface{},sending *sync.Mutex){
	sending.Lock()
	defer sending.Unlock()
	if err:=cc.Write(h,body);err!=nil{
		log.Println("rpc server: write response error: ",err)
	}
}

func (server *Server) handleRequest(cc codec.Codec,req *request,sending *sync.Mutex,wg *sync.WaitGroup){

	defer wg.Done()
	log.Println(req.h,req.argv.Elem())
	req.replyv=reflect.ValueOf(fmt.Sprintf("rpc resp %d",req.h.Seq))
	server.sendResponse(cc,req.h,req.replyv.Interface(),sending)
}