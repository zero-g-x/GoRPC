package gorpc

import (
	"GoRPC/codec"
	"encoding/json"
	"errors"
	"go/ast"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

type Server struct{
	serviceMap sync.Map
}

func (server *Server)Register(ins interface{})error{
	s:=newService(ins)
	if _,dup:=server.serviceMap.LoadOrStore(s.name,s);dup{
		return errors.New("service "+s.name+" alreaady defined")
	}
	return nil
}

func Register(ins interface{})error{
	return DefaultServer.Register(ins)
}

func (server *Server) findService(serviceMethod string)(*service, *methodType, error){
	var svc *service
	var mType *methodType
	var err error
	dot:=strings.LastIndex(serviceMethod,".")
	if dot<0{
		err=errors.New("invalid serviceMethod: "+serviceMethod)
		return svc,mType,err
	}
	serviceName,methodName:=serviceMethod[0:dot],serviceMethod[dot+1:]
	serviceIns,ok:=server.serviceMap.Load(serviceName)
	if !ok{
		err=errors.New("service "+serviceName+" not found")
		return svc,mType,err
	}
	svc=serviceIns.(*service)
	mType=svc.methods[methodName]
	//log.Println("methodType in findService=",mType)
	if mType==nil{
		err=errors.New("method "+methodName+" not found")
	}
	return svc,mType,err
} 

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
	mType *methodType
	svc *service//service of request
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
	//log.Println("h.ServiceMethod=",h.ServiceMethod)
	req.svc,req.mType,err=server.findService(h.ServiceMethod)
	//log.Println("svc= ",req.svc,",mType=",req.mType)
	if err!=nil{
		return req,err
	}

	req.argv=req.mType.newArgv()
	req.replyv=req.mType.newReplyv()

	argvi:=req.argv.Interface()
	//argvi should be a pointer
	if req.argv.Type().Kind()!=reflect.Ptr{
		argvi=req.argv.Addr().Interface() 
	}
	if err=cc.ReadBody(argvi);err!=nil{
		log.Println("rpc server: read argv error: ",err)
		return req,err
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



func (server *Server) handleRequest(cc codec.Codec,req *request,sendmu *sync.Mutex,wg *sync.WaitGroup){

	defer wg.Done()
	//log.Println(req.h,req.argv.Elem())
	//req.replyv=reflect.ValueOf(fmt.Sprintf("rpc resp %d",req.h.Seq))
	err:=req.svc.call(req.mType,req.argv,req.replyv)
	if err!=nil{
		req.h.Error=err.Error()
		invalidRequest:="request is not valid"
		server.sendResponse(cc,req.h,invalidRequest,sendmu)
	}
	server.sendResponse(cc,req.h,req.replyv.Interface(),sendmu)
}



type methodType struct{
	method reflect.Method
	ArgType reflect.Type
	ReplyType reflect.Type
	numCalls uint64
}

func (mt *methodType) NumCalls() uint64{
	return atomic.LoadUint64(&mt.numCalls)
}

func (mt *methodType) newArgv()  ( reflect.Value){
	var argv reflect.Value
	//log.Println("method type=",mt)
	if mt.ArgType.Kind()==reflect.Ptr{
		argv=reflect.New(mt.ArgType.Elem())
	}else{
		argv=reflect.New(mt.ArgType).Elem()
	}
	return argv
}

func (mt *methodType) newReplyv() ( reflect.Value){
	replyv:=reflect.New(mt.ReplyType.Elem())
	switch mt.ReplyType.Elem().Kind(){
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(mt.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(mt.ReplyType.Elem(),0,0))
	}
	return replyv
}

type service struct{
	name string //name of struct
	typ reflect.Type 
	ins reflect.Value//instance of struct
	methods map[string]*methodType
}

func invalidTypeName(t reflect.Type) bool{
	return !(ast.IsExported(t.Name())||t.PkgPath()=="")
}

func (s *service) registerMethods(){
	s.methods=make(map[string]*methodType)
	for i:=0;i<s.typ.NumMethod();i++{
		method:=s.typ.Method(i)
		//method(serviceName,arg,reply)
		mType:=method.Type
		if mType.NumIn()!=3 || mType.NumOut()!=1{
			continue//?
		}
		if mType.Out(0)!=reflect.TypeOf((*error)(nil)).Elem(){
			continue
		}
		argType,replyType:=mType.In(1),mType.In(2)
		if invalidTypeName(argType)||invalidTypeName(replyType){
			continue
		}
		s.methods[method.Name]=&methodType{
			method: method,
			ArgType: argType,
			ReplyType: replyType,
		}
		log.Println("server regerster method: ",s.name,".",method.Name,"method type=",s.methods[method.Name])
	}
}

func newService(ins interface{})( *service){
	s:=&service{}
	s.ins=reflect.ValueOf(ins)
	s.name=reflect.Indirect(s.ins).Type().Name()
	s.typ=reflect.TypeOf(ins)
	if !ast.IsExported(s.name){
		log.Fatalf("rpc server :%s is not an exported service name",s.name)
	}
	s.registerMethods()
	return s
}

func (s *service)call(mt *methodType,argv,replyv reflect.Value)error{
	atomic.AddUint64(&mt.numCalls,1)
	f := mt.method.Func
	returnV:=f.Call([]reflect.Value{s.ins,argv,replyv})
	intf:=returnV[0].Interface()
	if intf!=nil{
		return intf.(error)
	}
	return nil
}
