package source

import (
	"errors"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/receiver"
	"hash/fnv"
	"net"
	"sync"
)

type Source struct {
	name      string
	IP        net.IP
	receivers []*receiver.Receiver

	inputChannel chan *receiver.Message

	Config *config.SourceConfig

	error func(error)
	log   func(string)
}

func New(globalConfig *config.Config, Config *config.SourceConfig, errFunc func(error), logFunc func(string)) *Source {

	s := Source{
		name:         Config.Name,
		Config:       Config,
		IP:           net.ParseIP(Config.SourceIP),
		log:          logFunc,
		error:        errFunc,
		inputChannel: make(chan *receiver.Message, 10000),
	}

	if Config.StickyBytesLength > 0 {
		s.Config.StickyBytesEnd = s.Config.StickyBytesStart + s.Config.StickyBytesLength
	}

	return &s
}

func (s *Source) listen() {

	m, ok := <-s.inputChannel

	for ok {
		if s.Config.StickyBytesLength > 0 {

			stickySum := hash(m.Payload[s.Config.StickyBytesStart:s.Config.StickyBytesEnd])

			if err := s.receivers[stickySum%uint64(len(s.receivers))].AddMessage(m); err != nil {
				s.error(err)
			}
		} else {

			for _, r := range s.receivers {
				err := r.AddMessage(m)

				if err != nil {
					s.error(err)
				}
			}
		}

		m, ok = <-s.inputChannel
	}
}

func hash(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

func (s *Source) Listen(wg *sync.WaitGroup) error {

	wg.Add(1)

	go func() {
		s.listen()
		s.log(s.name + " - source stopped.") // This message might never make it out.
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

func (s *Source) Stop() {
	close(s.inputChannel)
}
