package pseudomultiplexer_test

import (
	"fmt"
	"testing"
	"time"

	multiplexer "github.com/threadfly/pseudo-multiplexer"
)

type TestHandler struct {
	c    *time.Ticker
	stop chan struct{}
}

func NewTestHandler() *TestHandler {
	return &TestHandler{
		c:    time.NewTicker(5 * time.Second),
		stop: make(chan struct{}, 1),
	}
}

func (this *TestHandler) Chan() interface{} {
	return this.c.C
}

func (this *TestHandler) Run(v interface{}) {
	fmt.Printf("[TestHandler][Run] call...time:%s\n", v.(time.Time).String())
}

func (this *TestHandler) Stop() {
	fmt.Printf("[TestHandler][Stop] call...\n")
	this.stop <- struct{}{}
}

func (this *TestHandler) Wait() {
	fmt.Printf("[TestHandler][Wait] call...\n")
	<-this.stop
}

func Test_PseudoMultiplexer(T *testing.T) {
	h := NewTestHandler()
	multiPlexer := multiplexer.NewPseudoMultiPlexer()
	err := multiPlexer.Register(h)
	if err != nil {
		fmt.Printf("register err:%s \n", err)
		return
	}

	go func() {
		time.Sleep(15 * time.Second)
		x := NewTestHandler()
		err := multiPlexer.Register(x)
		if err != nil {
			fmt.Printf("register err:%s \n", err)
			return
		}
		time.Sleep(15 * time.Second)
		err = multiPlexer.UnRegister(x)
		multiPlexer.Stop()
		fmt.Printf("stop multiPlexer\n")
	}()

	h.Wait()
	time.Sleep(5 * time.Second)
}
