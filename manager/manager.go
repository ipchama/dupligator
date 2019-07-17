package manager

import (
	"encoding/binary"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/receiver"
	"github.com/ipchama/dupligator/source"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

type Manager struct {
	config    *config.Config
	waitGroup sync.WaitGroup
	/*
		Even with all conversions and length checks needed,
		benchmarks showed these key choices as being more than 8x faster than using IP.String() for keys.
	*/
	sourceMapV4  map[uint32]*source.Source
	sourceV4All  *source.Source
	sourceMapV6  map[[16]byte]*source.Source
	sourceV6All  *source.Source
	receiverMap  map[string]*receiver.Receiver
	v4Conn       net.Conn
	v6Conn       net.Conn
	logChannel   chan *LogMessage
	errorChannel chan *LogMessage
	stopping     uint32
}

type LogMessage struct {
	Message string
	Stop    bool
}

func New(globalConfig *config.Config) *Manager {

	m := Manager{
		config:      globalConfig,
		sourceMapV4: make(map[uint32]*source.Source),
		sourceMapV6: make(map[[16]byte]*source.Source),

		sourceV4All: nil,
		sourceV6All: nil,

		receiverMap: make(map[string]*receiver.Receiver),

		logChannel:   make(chan *LogMessage, 1000),
		errorChannel: make(chan *LogMessage, 1000),
	}

	return &m
}

func (m *Manager) Init() error {

	// Load and init receivers
	for i := 0; i < len(m.config.Receivers); i++ {
		newReceiver := receiver.New(m.config, &m.config.Receivers[i], m.recordError, m.recordLog)
		m.receiverMap[m.config.Receivers[i].Name] = newReceiver
	}

	// Load and init sources
	for i := 0; i < len(m.config.Sources); i++ {
		newSource := source.New(m.config, &m.config.Sources[i], m.recordError, m.recordLog)

		if m.config.Sources[i].SourceIP == "0.0.0.0" {
			m.recordLog("Adding IPv4 'any' source.")
			m.sourceV4All = newSource

		} else if m.config.Sources[i].SourceIP == "::/0" {
			m.recordLog("Adding IPv6 'any' source.")
			m.sourceV6All = newSource
		}

		/* Even for "any" sources, we can still let them get added to the map so that the init and deinit can be handled the same for them as the rest. */

		if newSource.IP.To4() != nil {

			if len(newSource.IP) == 16 { // Could be v4 expressed as v6
				m.sourceMapV4[binary.BigEndian.Uint32(newSource.IP[12:16])] = newSource
			} else {
				m.sourceMapV4[binary.BigEndian.Uint32(newSource.IP)] = newSource
			}

		} else {
			var v6Bytes [16]byte
			copy(v6Bytes[:], newSource.IP)
			m.sourceMapV6[v6Bytes] = newSource
		}

		for _, sourceReceiver := range m.config.Sources[i].Receivers {
			newSource.AddReceiver(m.receiverMap[sourceReceiver])
		}
	}

	return nil
}

func (m *Manager) Run() error {
	var err error

	// Start up the log channel reader
	m.waitGroup.Add(1)
	go func() {
		var msg *LogMessage
		for {
			msg = <-m.logChannel

			if msg.Stop {
				break
			}

			log.Printf("INFO: %s", msg.Message)
		}

		m.waitGroup.Done()
	}()

	// Start up the error channel reader
	m.waitGroup.Add(1)
	go func() {
		var msg *LogMessage
		for {
			msg = <-m.errorChannel
			if msg.Stop {
				break
			}

			log.Printf("ERROR: %s", msg.Message)
		}
		m.waitGroup.Done()
	}()

	/*
		Start sources and receivers
	*/

	for _, r := range m.receiverMap {
		err = r.StartSending(&m.waitGroup)

		if err != nil {
			return err
		}
	}

	for _, s := range m.sourceMapV4 {
		err = s.Listen(&m.waitGroup)

		if err != nil {
			return err
		}
	}

	for _, s := range m.sourceMapV6 {
		err = s.Listen(&m.waitGroup)

		if err != nil {
			return err
		}
	}

	/*
		Start packet listeners
	*/
	err = m.runV4()

	if err != nil {
		return err
	}

	err = m.runV6()

	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) runV4() error {
	// V4
	conn4, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.ParseIP(m.config.LocalV4Config.Address),
		Port: m.config.LocalV4Config.Port,
	})

	if err != nil {
		return err
	}

	m.v4Conn = conn4

	m.waitGroup.Add(1)
	go func() {

		data := make([]byte, 4096)
		var i uint32

		for {
			read, remoteAddr, err := conn4.ReadFromUDP(data)

			if err != nil {
				if atomic.LoadUint32(&m.stopping) == 0 { // Suppress errors that very likely occurred because the conn was closed by a call to Stop()
					m.recordError(err)
				}

				break
			}

			if len(remoteAddr.IP) == 16 {
				i = binary.BigEndian.Uint32(remoteAddr.IP[12:16])
			} else {
				i = binary.BigEndian.Uint32(remoteAddr.IP)
			}

			if m.sourceV4All != nil {
				msg := &receiver.Message{
					SourceAddress: remoteAddr.IP,
					SourcePort:    remoteAddr.Port,
					Payload:       data[:read],
				}

				err = m.sourceV4All.AddMessage(msg)

				if err != nil {
					m.recordError(err)
				}
			}

			if s, ok := m.sourceMapV4[i]; ok {

				msg := &receiver.Message{
					SourceAddress: remoteAddr.IP,
					SourcePort:    remoteAddr.Port,
					Payload:       data[:read],
				}

				err = s.AddMessage(msg)

				if err != nil {
					m.recordError(err)
				}
			}
		}
		m.waitGroup.Done()
	}()

	return nil
}

func (m *Manager) runV6() error {
	conn6, err := net.ListenUDP("udp6", &net.UDPAddr{
		IP:   net.ParseIP(m.config.LocalV6Config.Address),
		Port: m.config.LocalV6Config.Port,
	})

	if err != nil {
		return err
	}

	m.v6Conn = conn6

	m.waitGroup.Add(1)
	go func() {
		data := make([]byte, 4096)
		var v6Bytes [16]byte

		for {
			read, remoteAddr, err := conn6.ReadFromUDP(data)

			if err != nil {
				if atomic.LoadUint32(&m.stopping) == 0 { // Suppress errors that very likely occurred because the conn was closed by a call to Stop()
					m.recordError(err)
				}
				break
			}

			if m.sourceV6All != nil {
				msg := &receiver.Message{
					SourceAddress: remoteAddr.IP,
					SourcePort:    remoteAddr.Port,
					Payload:       data[:read],
				}

				if err = m.sourceV6All.AddMessage(msg); err != nil {
					m.recordError(err)
				}
			}

			copy(v6Bytes[:], remoteAddr.IP)
			if s, ok := m.sourceMapV6[v6Bytes]; ok {

				msg := &receiver.Message{
					SourceAddress: remoteAddr.IP,
					SourcePort:    remoteAddr.Port,
					Payload:       data[:read],
				}

				err = s.AddMessage(msg)

				if err != nil {
					m.recordError(err)
				}
			}
		}

		m.waitGroup.Done()
	}()

	return nil
}

func (m *Manager) Stop() {

	/*
		Send stop message to sources and receivers.
		Could construct things so that sources pass along the stop message to receivers, but the
		idea is that receivers could be shared by many sources; in which case, multiple stop
		messages would be sent to the same reciever.  Not the end of the world, just seems unnecessary
		since we can just send messages to everyone here.
	*/

	atomic.StoreUint32(&m.stopping, 1)

	err := m.v4Conn.Close()
	if err != nil {
		m.recordError(err)
	}

	err = m.v6Conn.Close()
	if err != nil {
		m.recordError(err)
	}

	stop := &receiver.Message{
		Stop: true,
	}

	for _, s := range m.sourceMapV4 {
		s.AddMessage(stop)
	}

	for _, s := range m.sourceMapV6 {
		s.AddMessage(stop)
	}

	for _, r := range m.receiverMap {
		r.AddMessage(stop)
	}

	/*
		Stop logging.
	*/

	m.logChannel <- &LogMessage{Stop: true}
	m.errorChannel <- &LogMessage{Stop: true}

	m.waitGroup.Wait()
}

func (m *Manager) recordLog(str string) {
	msg := &LogMessage{
		Message: str,
	}
	select {
	case m.logChannel <- msg:
	default:
	}
}

func (m *Manager) recordError(err error) {
	msg := &LogMessage{
		Message: err.Error(),
	}

	select {
	case m.errorChannel <- msg:
	default:
	}
}
