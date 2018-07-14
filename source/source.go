package source

import (
	"errors"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/receiver"
	"net"
	"sync"
)

type Source struct {
	name      string
	IP        net.IP
	receivers []*receiver.Receiver

	inputChannel chan *receiver.Message

	error func(error)
	log   func(string)
}

func New(globalConfig *config.Config, myConfig *config.SourceConfig, errFunc func(error), logFunc func(string)) *Source {

	s := Source{
		name:         myConfig.Name,
		IP:           net.ParseIP(myConfig.SourceIP),
		log:          logFunc,
		error:        errFunc,
		inputChannel: make(chan *receiver.Message, 10000),
	}

	return &s
}

// TODO: Need some condition on the for-loop and a Stop() method.

func (s *Source) listen() {
	var m *receiver.Message
	for {
		m = <-s.inputChannel
		for _, r := range s.receivers {
			r.AddMessage(m)
		}
	}
}

func (s *Source) Listen(wg *sync.WaitGroup) error {

	wg.Add(1)

	go func() {
		s.listen()
		wg.Done()
	}()

	s.log(s.name + " - source listening.")

	return nil
}

func (s *Source) AddReceiver(r *receiver.Receiver) {
	s.receivers = append(s.receivers, r)
}

func (s *Source) AddMessage(p []byte, sourceAddress net.IP, sourcePort int) error {

	msg := &receiver.Message{
		SourceAddress: sourceAddress,
		SourcePort:    sourcePort,
		Payload:       p,
	}

	select {
	case s.inputChannel <- msg:
	default:
		return errors.New("Source channel is full.")
	}

	return nil
}
