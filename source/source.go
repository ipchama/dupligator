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

func (s *Source) listen() {
	var m *receiver.Message

	for {
		m = <-s.inputChannel

		// Break before sending to receivers.  The manager will handle stopping them.
		if m.Stop {
			break
		}

		for _, r := range s.receivers {
			err := r.AddMessage(m)

			if err != nil {
				s.error(err)
			}
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

func (s *Source) AddMessage(msg *receiver.Message) error {

	select {
	case s.inputChannel <- msg:
	default:
		return errors.New("Source channel is full.")
	}

	return nil
}
