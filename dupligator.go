package main

import (
	// "errors"
	"encoding/binary"
	"flag"
	"github.com/go-yaml/yaml"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/receiver"
	"github.com/ipchama/dupligator/source"
	"io/ioutil"
	"log"
	"net"
	"sync"
)

func main() {

	var waitGroup sync.WaitGroup

	receiverMap := make(map[string]*receiver.Receiver)
	/*
		Even with all conversions and length checks needed,
		benchmarks showed these key choices as being more than 8x faster than using IP.String() for keys.
	*/
	sourceMapV4 := make(map[uint32]*source.Source)
	sourceMapV6 := make(map[[16]byte]*source.Source)

	logChannel := make(chan string, 1000)
	errorChannel := make(chan error, 1000)

	configPathPtr := flag.String("config", "./config.yml", "A yaml file containing config options.")
	flag.Parse()

	configData, err := ioutil.ReadFile(*configPathPtr)

	if err != nil {
		panic(err)
	}

	config := &config.Config{}

	err = yaml.Unmarshal(configData, config)

	if err != nil {
		panic(err)
	}

	for i := 0; i < len(config.Receivers); i++ {
		newReceiver := receiver.New(config, &config.Receivers[i], func(perr error) { recordError(perr, errorChannel) }, func(msg string) { recordLog(msg, logChannel) })
		receiverMap[config.Receivers[i].Name] = newReceiver

		err = newReceiver.StartSending(&waitGroup)

		if err != nil {
			panic(err)
		}
	}

	for i := 0; i < len(config.Sources); i++ {
		newSource := source.New(config, &config.Sources[i], func(perr error) { recordError(perr, errorChannel) }, func(msg string) { recordLog(msg, logChannel) })

		if newSource.IP.To4() != nil {

			if len(newSource.IP) == 16 { // Could be v4 expressed as v6
				sourceMapV4[binary.BigEndian.Uint32(newSource.IP[12:16])] = newSource
			} else {
				sourceMapV4[binary.BigEndian.Uint32(newSource.IP)] = newSource
			}

		} else {
			var v6Bytes [16]byte
			copy(v6Bytes[:], newSource.IP)
			sourceMapV6[v6Bytes] = newSource
		}

		for _, sourceReceiver := range config.Sources[i].Receivers {
			newSource.AddReceiver(receiverMap[sourceReceiver])
		}

		err = newSource.Listen(&waitGroup)

		if err != nil {
			panic(err)
		}
	}

	/*
		Start log and error readers
	*/

	// Start up the log channel reader
	waitGroup.Add(1)
	go func() {
		var msg string
		for {
			msg = <-logChannel
			log.Printf("INFO: %s", msg)
		}

		waitGroup.Done() // TODO: Need some condition on the for-loop to make sure we can even hit this.
	}()

	// Start up the error channel reader
	waitGroup.Add(1)
	go func() {
		var err error
		for {
			err = <-errorChannel
			log.Printf("ERROR: %s", err.Error())
		}
		waitGroup.Done() // TODO: Need some condition on the for-loop to make sure we can even hit this.
	}()

	/*
		Start listeners
	*/

	// V4
	conn4, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.ParseIP(config.LocalV4Config.Address),
		Port: config.LocalV4Config.Port,
	})

	if err != nil {
		panic(err)
	}

	waitGroup.Add(1)
	go func() {

		data := make([]byte, 4096)
		var i uint32

		for {
			read, remoteAddr, err := conn4.ReadFromUDP(data)

			if err != nil {
				recordError(err, errorChannel)
				break
			}

			if len(remoteAddr.IP) == 16 {
				i = binary.BigEndian.Uint32(remoteAddr.IP[12:16])
			} else {
				i = binary.BigEndian.Uint32(remoteAddr.IP)
			}

			if s, ok := sourceMapV4[i]; ok {

				err = s.AddMessage(data[:read], remoteAddr.IP, remoteAddr.Port)

				if err != nil {
					recordError(err, errorChannel)
				}
			}
		}
		waitGroup.Done()
	}()

	// V6
	conn6, err := net.ListenUDP("udp6", &net.UDPAddr{
		IP:   net.ParseIP(config.LocalV6Config.Address),
		Port: config.LocalV6Config.Port,
	})

	if err != nil {
		panic(err)
	}

	waitGroup.Add(1)
	go func() {
		data := make([]byte, 4096)
		var v6Bytes [16]byte

		for {
			read, remoteAddr, err := conn6.ReadFromUDP(data)

			if err != nil {
				recordError(err, errorChannel)
				break
			}

			copy(v6Bytes[:], remoteAddr.IP)
			if s, ok := sourceMapV6[v6Bytes]; ok {

				err = s.AddMessage(data[:read], remoteAddr.IP, remoteAddr.Port)

				if err != nil {
					recordError(err, errorChannel)
				}
			}
		}

		waitGroup.Done()
	}()

	/*
		TODO:
		Need to know if any read errors caused the primary read functions to return
		If so, need to stop the rest of the go routines by calling stops on the sources and then stops on the receivers.
		Sources should probably also check their receivers to make sure they are running before giving them messages just to
		avoid a bunch of dumb memory usage from filled channels for receivers that stopped for some reason.
	*/

	waitGroup.Wait()

}

func recordLog(str string, logChannel chan string) {
	select {
	case logChannel <- str:
	default:
	}
}

func recordError(err error, errorChannel chan error) {
	select {
	case errorChannel <- err:
	default:
	}
}
