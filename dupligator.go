package main

import (
	"flag"
	"github.com/go-yaml/yaml"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/manager"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
)

type SignalType int

const (
	stopSignalType SignalType = iota
	reloadSignalType
)

func main() {

	internalSignalsChann := make(chan SignalType)

	configPathPtr := flag.String("config", "./config.yml", "A yaml file containing config options.")
	flag.Parse()

	catchSignals(internalSignalsChann)

	for {

		configData, err := ioutil.ReadFile(*configPathPtr)

		if err != nil {
			panic(err)
		}

		config := &config.Config{}

		err = yaml.Unmarshal(configData, config)

		if err != nil {
			panic(err)
		}

		streamManager := manager.New(config)

		err = streamManager.Init()

		if err != nil {
			panic(err)
		}

		err = streamManager.Run()

		if err != nil {
			panic(err)
		}

		iSig := <-internalSignalsChann

		if iSig == stopSignalType {
			println("Attempting graceful shutdown...")
			streamManager.Stop()
			break
		} else if iSig == reloadSignalType {
			println("Attempting graceful reload...")
			streamManager.Stop()
		}
	}
}

func catchSignals(iChan chan SignalType) {
	osSignalsChann := make(chan os.Signal)
	signal.Notify(osSignalsChann, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		var caughtSig os.Signal
		for ; ; caughtSig = <-osSignalsChann {
			if caughtSig == syscall.SIGTERM || caughtSig == syscall.SIGINT {
				iChan <- stopSignalType
				break
			} else if caughtSig == syscall.SIGHUP {
				iChan <- reloadSignalType
			}
		}
	}()

}
