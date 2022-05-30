package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int 

const(
	RandomSelect SelectMode = iota
	RoundRobinSelect 
)

type Discovery interface{
	//update service list from register center
	Refresh() error 
	//update servers dynamically
	Update(servers []string) error
	//get a server under selected mode
	Get(mode SelectMode)(string,error)
	GetAll()([]string,error)
}

//check Discovery implements MultiServersDisc
var _ Discovery=(*MultiServersDiscovery)(nil)

//for multi servers without a registry center
type MultiServersDiscovery struct{
	r *rand.Rand // to get random number
	mu sync.RWMutex
	servers []string 
	index int //for robin akgorithm: record selected position
}

func NewMultiServersDiscovery(servers []string) *MultiServersDiscovery{
	d:=&MultiServersDiscovery{
		servers: servers,
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	d.index=d.r.Intn(math.MaxInt32-1)
	return d
}

func (d *MultiServersDiscovery)Refresh()error{
	return nil
}

func (d *MultiServersDiscovery)Update(servers []string)error{
	d.mu.Lock()
	defer d.mu.Unlock()
	d.servers=servers
	return nil
}

func (d *MultiServersDiscovery)Get(mode SelectMode)(string,error){
	d.mu.Lock()
	defer d.mu.Unlock()
	n:=len(d.servers)
	if n==0{
		return "",errors.New("get server error: no available server")
	}
	switch mode{
	case RandomSelect:
		return d.servers[d.r.Intn(n)],nil
	case RoundRobinSelect:
		s:=d.servers[d.index%n]
		d.index=(d.index+1)%n
		return s,nil
	default:
		return "",errors.New("get server unsupported select mode")
	}
}

func (d *MultiServersDiscovery)GetAll()([]string,error){
	d.mu.RLock()
	defer d.mu.RUnlock()
	servers:=make([]string,len(d.servers),len(d.servers))
	copy(servers,d.servers)
	return servers,nil
}