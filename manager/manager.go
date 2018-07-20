package manager

import (
	"encoding/binary"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/receiver"
	"github.com/ipchama/dupligator/source"
	"log"
	"net"
	"sync"
)

type Manager struct {
	config    *config.Config
	waitGroup sync.WaitGroup
	/*
		Even with all conversions and length checks needed,
		benchmarks showed these key choices as being more than 8x faster than using IP.String() for keys.
	*/
	sourceMapV4  map[uint32]*source.Source
	sourceMapV6  map[[16]byte]*source.Source
	receiverMap  map[string]*receiver.Receiver
	logChannel   chan string
	errorChannel chan error
}

func New(globalConfig *config.Config) *Manager {

	m := Manager{
		config:      globalConfig,
		sourceMapV4: make(map[uint32]*source.Source),
		sourceMapV6: make(map[[16]byte]*source.Source),

		receiverMap: make(map[string]*receiver.Receiver),

		logChannel:   make(chan string, 1000),
		errorChannel: make(chan error, 1000),
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
		var msg string
		for {
			msg = <-m.logChannel
			log.Printf("INFO: %s", msg)
		}

		m.waitGroup.Done() // TODO: Need some condition on the for-loop to make sure we can even hit this.
	}()

	// Start up the error channel reader
	m.waitGroup.Add(1)
	go func() {
		var err error
		for {
			err = <-m.errorChannel
			log.Printf("ERROR: %s", err.Error())
		}
		m.waitGroup.Done() // TODO: Need some condition on the for-loop to make sure we can even hit this.
	}()

	/*
		Start sources and recievers
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

	/*
		TODO:
		Need to know if any read errors caused the primary read functions to return
		If so, need to stop the rest of the go routines by calling stops on the sources and then stops on the receivers.
		Sources should probably also check their receivers to make sure they are running before giving them messages just to
		avoid a bunch of dumb memory usage from filled channels for receivers that stopped for some reason.
	*/
	m.waitGroup.Wait()

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

	m.waitGroup.Add(1)
	go func() {

		data := make([]byte, 4096)
		var i uint32

		for {
			read, remoteAddr, err := conn4.ReadFromUDP(data)

			if err != nil {
				m.recordError(err)
				break
			}

			if len(remoteAddr.IP) == 16 {
				i = binary.BigEndian.Uint32(remoteAddr.IP[12:16])
			} else {
				i = binary.BigEndian.Uint32(remoteAddr.IP)
			}

			if s, ok := m.sourceMapV4[i]; ok {

				err = s.AddMessage(data[:read], remoteAddr.IP, remoteAddr.Port)

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

	m.waitGroup.Add(1)
	go func() {
		data := make([]byte, 4096)
		var v6Bytes [16]byte

		for {
			read, remoteAddr, err := conn6.ReadFromUDP(data)

			if err != nil {
				m.recordError(err)
				break
			}

			copy(v6Bytes[:], remoteAddr.IP)
			if s, ok := m.sourceMapV6[v6Bytes]; ok {

				err = s.AddMessage(data[:read], remoteAddr.IP, remoteAddr.Port)

				if err != nil {
					m.recordError(err)
				}
			}
		}

		m.waitGroup.Done()
	}()

	return nil
}

func (m *Manager) recordLog(str string) {
	select {
	case m.logChannel <- str:
	default:
	}
}

func (m *Manager) recordError(err error) {
	select {
	case m.errorChannel <- err:
	default:
	}
}
