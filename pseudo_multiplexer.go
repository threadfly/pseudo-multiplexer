package pseudomultiplexer

import (
	"fmt"
	"reflect"
	"sync"
)

const (
	multiPlexerHandlerInitLen = 0x4
)

type multiPlexerStatus bool

const (
	multiPlexerRuning multiPlexerStatus = true
	multiPlexerStop   multiPlexerStatus = false
)

type multiPlexerSentinelCmd int

const (
	cmdStop multiPlexerSentinelCmd = 0x1
)

type PseudoMultiPlexerHandler interface {
	Chan() interface{}
	Run(interface{})
	Stop()
}

type multiPlexerSentinel struct {
	isClose     bool
	isCloseMtx  *sync.RWMutex
	proxy       chan struct{}
	sentinel    chan multiPlexerSentinelCmd
	multiPlexer *PseudoMultiPlexer
}

func newMultiPlexerSentinel(m *PseudoMultiPlexer) *multiPlexerSentinel {
	return &multiPlexerSentinel{
		isCloseMtx:  new(sync.RWMutex),
		proxy:       make(chan struct{}, 1),
		sentinel:    make(chan multiPlexerSentinelCmd, 1),
		multiPlexer: m,
	}
}

// help gc
func (this *multiPlexerSentinel) clear() {
	this.multiPlexer = nil
	this.sentinel = nil
	this.proxy = nil
}

func (this *multiPlexerSentinel) setClose(t bool) {
	this.isCloseMtx.RLock()
	defer this.isCloseMtx.RUnlock()
	this.isClose = t
}

func (this *multiPlexerSentinel) getClose() bool {
	this.isCloseMtx.RLock()
	defer this.isCloseMtx.RUnlock()
	return this.isClose
}

func (this *multiPlexerSentinel) Chan() interface{} {
	return this.sentinel
}

func (this *multiPlexerSentinel) Run(v interface{}) {
	cmd, ok := v.(multiPlexerSentinelCmd)
	if !ok {
		panic("type assert failed...")
	}

	switch cmd {
	case cmdStop:
		this.multiPlexer.setRunning(multiPlexerStop)
	}
}

func (this *multiPlexerSentinel) Stop() {
	this.multiPlexer.setRunning(multiPlexerStop)
}

func (this *multiPlexerSentinel) terminalMultiPlexer() {
	close(this.sentinel)
}

func (this *multiPlexerSentinel) terminalProxy() {
	close(this.proxy)
}

func (this *multiPlexerSentinel) stopMultiPlexer() {
	this.sentinel <- cmdStop
}

func (this *multiPlexerSentinel) waitStopMultiPlexer() {
	<-this.proxy
}

func (this *multiPlexerSentinel) finishStopMultiPlexer() {
	this.proxy <- struct{}{}
}

type PseudoMultiPlexer struct {
	sentinel    *multiPlexerSentinel
	isRuning    multiPlexerStatus
	isRuningMtx *sync.RWMutex
	handlers    []PseudoMultiPlexerHandler
	handlerMtx  *sync.RWMutex
}

func NewPseudoMultiPlexer() *PseudoMultiPlexer {
	m := &PseudoMultiPlexer{
		sentinel:    nil,
		isRuning:    multiPlexerStop,
		isRuningMtx: new(sync.RWMutex),
		handlers:    make([]PseudoMultiPlexerHandler, 0, multiPlexerHandlerInitLen),
		handlerMtx:  new(sync.RWMutex),
	}

	m.sentinel = newMultiPlexerSentinel(m)
	m.sentinel.setClose(false)
	m.Register(m.sentinel)
	return m
}

func PseudoMultiPlexerHandlerCheck(h PseudoMultiPlexerHandler) error {
	if h == nil {
		return fmt.Errorf("handler is nil")
	}

	typ := reflect.TypeOf(h.Chan())
	if typ.Kind() != reflect.Chan {
		return fmt.Errorf("Chan method have return chan type")
	}

	if typ.ChanDir()&reflect.RecvDir == 0 {
		return fmt.Errorf("chan type only allow send")
	}

	return nil
}

func (this *PseudoMultiPlexer) run() {
	this.setRunning(true)

	cases := make([]reflect.SelectCase, len(this.handlers))
	for i, ch := range this.handlers {
		cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ch.Chan())}
	}

	go func() {
		fmt.Printf("[PseudoMultiPlexer][run] start... \n")
		for {
			chosen, value, ok := reflect.Select(cases)
			///ok will be true if the channel has not been closed.
			if len(cases) < chosen {
				panic("handler index exceed ")
			}

			if ok {
				fmt.Printf("[PseudoMultiPlexer][run] case chosen:%d ok:%t type:%s\n", chosen, ok, value.Type().String())
			} else {
				fmt.Printf("[PseudoMultiPlexer][run] case chosen:%d ok:%t receive close signal\n", chosen, ok)
			}
			if chosen == 0 {
				if ok {
					this.handlers[0].Run(value.Interface())
				} else {
					this.handlers[0].Stop()
				}
			} else {
				if ok {
					go this.handlers[chosen].Run(value.Interface())
				} else {
					go this.handlers[chosen].Stop()
				}
			}

			if !this.getRunning() {
				break
			}
		}
		this.sentinel.finishStopMultiPlexer()
		fmt.Printf("[PseudoMultiPlexer][run] stop... \n")
		// TODO ...
	}()
}

func (this *PseudoMultiPlexer) setRunning(v multiPlexerStatus) {
	this.isRuningMtx.Lock()
	defer this.isRuningMtx.Unlock()
	this.isRuning = v
}

func (this *PseudoMultiPlexer) getRunning() bool {
	this.isRuningMtx.RLock()
	defer this.isRuningMtx.RUnlock()
	return bool(this.isRuning)
}

func (this *PseudoMultiPlexer) Register(h PseudoMultiPlexerHandler) (err error) {
	if this.sentinel.getClose() {
		return nil
	}

	err = PseudoMultiPlexerHandlerCheck(h)
	if err != nil {
		return err
	}

	if this.getRunning() {
		this.sentinel.stopMultiPlexer()
		this.addHandler(h)
		this.sentinel.waitStopMultiPlexer()
	} else {
		this.addHandler(h)
	}
	this.run()
	return
}

func (this *PseudoMultiPlexer) UnRegister(h PseudoMultiPlexerHandler) (err error) {
	if this.sentinel.getClose() {
		return nil
	}

	err = PseudoMultiPlexerHandlerCheck(h)
	if err != nil {
		return err
	}

	if this.getRunning() {
		this.sentinel.stopMultiPlexer()
		this.rmHandler(h)
		this.sentinel.waitStopMultiPlexer()
	} else {
		this.rmHandler(h)
	}
	this.run()
	return
}

func (this *PseudoMultiPlexer) addHandler(h PseudoMultiPlexerHandler) {
	this.handlerMtx.Lock()
	defer this.handlerMtx.Unlock()
	this.handlers = append(this.handlers, h)
}

func (this *PseudoMultiPlexer) rmHandler(h PseudoMultiPlexerHandler) {
	this.handlerMtx.Lock()
	defer this.handlerMtx.Unlock()
	for i, hdl := range this.handlers {
		if reflect.DeepEqual(h, hdl) {
			this.handlers = append(this.handlers[:i], this.handlers[i+1:]...)
			break
		}
	}
}

// currency no safe
func (this *PseudoMultiPlexer) Stop() {
	if this.sentinel.getClose() {
		return
	}

	if this.getRunning() {
		this.sentinel.setClose(true)
		this.sentinel.terminalMultiPlexer()
		this.sentinel.waitStopMultiPlexer()
		this.handlerMtx.Lock()
		defer this.handlerMtx.Unlock()
		for _, handler := range this.handlers {
			handler.Stop()
		}
		this.sentinel.terminalProxy()
		this.setRunning(multiPlexerStop)
	}
}

// help gc
func (this *PseudoMultiPlexer) clear() {
	this.sentinel.clear()
	this.handlers = nil
	this.handlerMtx = nil
	this.isRuningMtx = nil
	this.sentinel = nil
}
