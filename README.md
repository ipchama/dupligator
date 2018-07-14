# DupliGator

DupliGator is a UDP packet replicator inspired by Samplicator (https://github.com/sleinen/samplicator).

It currently supports UDP-to-UDP (with source spoofing), UDP-to-TCP, and both IPv4 & IPv6.

Sources and receivers with differing IP versions can be combined. I.e., Payloads can come in via IPv4 and UDP but be sent out over IPv4 or IPv6 with UDP or TCP. The only requirement is that packets come in from UDP, but this can easily be changed in the future.

The only limitation is on source spoofing, which is only permitted with UDP, and only between matching IP versions.


## Getting Started

Just download, compile, and run.

### Prerequisites

* https://github.com/google/gopacket
* https://github.com/google/gopacket/layers
* https://github.com/go-yaml/yaml

```
go get github.com/google/gopacket
go get github.com/google/gopacket/layers
go get github.com/go-yaml/yaml
```

### Installing

```
go get github.com/ipchama/dupligator
go build dupligator.go
```

## Contributing

Contributions are welcome.

DupliGator went from idea to completion in a few hours, and so there plenty of TODOs, points that need improvement, and features that can easily be added (TLS and authentication options for TCP?)

## Versioning
Haven't decided. :)

## Authors

* **IPCHama** - *Initial work* - [ipchama](https://github.com/ipchama)

## License

This project is licensed under the GPL v3 License - see the [LICENSE.md](LICENSE.md) file for details

## Acknowledgments

* The awesome gopackets library
* Samplicator (https://github.com/sleinen/samplicator)


