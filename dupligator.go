package main

import (
	"flag"
	"github.com/go-yaml/yaml"
	"github.com/ipchama/dupligator/config"
	"github.com/ipchama/dupligator/manager"
	"io/ioutil"
)

func main() {

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

	streamManager := manager.New(config)

	err = streamManager.Init()

	if err != nil {
		panic(err)
	}

	err = streamManager.Run()

	if err != nil {
		panic(err)
	}

}
