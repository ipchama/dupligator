package receiver

import (
	"errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/ipchama/dupligator/config"
	"net"
	"strconv"
	"sync"
	"syscall"
)

type Message struct {
	SourceAddress net.IP
	SourcePort    int
	Payload       []byte
}

type Receiver struct {
	globalConfig             *config.Config
	config                   *config.ReceiverConfig
	name                     string
	ip                       net.IP
	isIPv4                   bool
	localGatewayHardwareAddr net.HardwareAddr
	localIp                  net.IP
	port                     int
	proto                    string
	spoof                    bool
	error                    func(error)
	log                      func(string)
	localInterface           *net.Interface
	inputChannel             chan *Message
	outputPath               interface{}
	done                     chan struct{}
}

func New(globalConfig *config.Config, myConfig *config.ReceiverConfig, errFunc func(error), logFunc func(string)) *Receiver {

	r := Receiver{
		globalConfig: globalConfig,
		config:       myConfig,
		name:         myConfig.Name,
		proto:        myConfig.Proto,
		port:         myConfig.Port,
		spoof:        myConfig.Spoof,
		log:          logFunc,
		error:        errFunc,
		inputChannel: make(chan *Message, 10000),
		done:         make(chan struct{}),
	}

	return &r
}

func (r *Receiver) init() (err error) {

	if r.config.IPvPref != 4 && r.config.IPvPref != 6 {
		r.config.IPvPref = 6
	}

	// Handling this here instead of just letting Dial do the work because configs that use spoofing will force us to do this kind of look-up anyway, so we might as well keep everything consistent.

	ips, err := net.LookupIP(r.config.Ip) // Can handle a hostname or an IP equally gracefully.

	if err != nil {
		return err
	}

	if len(ips) > 0 {
		r.ip = ips[0] // Default.  If there's only one IP or no IPs match the preference, this won't change.
		for _, ip := range ips {
			if (r.config.IPvPref == 4 && ip.To4() != nil) || (r.config.IPvPref == 6 && ip.To4() == nil) {
				r.ip = ip
				break
			}
		}
	} else {
		return errors.New("No records found for host: " + r.config.Ip)
	}

	r.isIPv4 = true

	remoteAddrString := r.ip.String()

	localInterfaceName := r.globalConfig.LocalV4Config.Interface

	if r.ip.To4() == nil {
		r.isIPv4 = false
		localInterfaceName = r.globalConfig.LocalV6Config.Interface
		r.localGatewayHardwareAddr, err = net.ParseMAC(r.globalConfig.LocalV6Config.GatewayMAC)
		remoteAddrString = "[" + remoteAddrString + "]"
	} else {
		r.localGatewayHardwareAddr, err = net.ParseMAC(r.globalConfig.LocalV4Config.GatewayMAC)
	}

	if err != nil {
		return err
	}

	if r.proto == "udp" {

		if !r.spoof { // Silly way to auto-select a local IP for use late while constructing UDP packets so the same port can be used for multiple receivers.
			connString := remoteAddrString + ":" + strconv.Itoa(r.port)

			if conn, err := net.Dial(r.proto, connString); err != nil {
				return err
			} else {
				r.localIp = conn.LocalAddr().(*net.UDPAddr).IP
			}
		}

		r.localInterface, err = net.InterfaceByName(localInterfaceName)

		if err != nil {
			return err
		}

		fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, syscall.ETH_P_ALL)

		if err != nil {
			return err
		}

		var haddr [8]byte
		copy(haddr[0:7], r.localInterface.HardwareAddr[0:7])
		addr := syscall.SockaddrLinklayer{
			Protocol: 0x008, // LSB ETH_P_IP (0x800)
			Ifindex:  r.localInterface.Index,
			Halen:    uint8(len(r.localInterface.HardwareAddr)),
			Addr:     haddr,
		}

		if !r.isIPv4 {
			addr.Protocol = 0x86dd // LSB ETH_P_IPV6 (0xdd68)
		}

		err = syscall.Bind(fd, &addr)

		if err != nil {
			return err
		}

		r.outputPath = fd

		r.log(r.name + " - Raw socket ready.")

	} else if r.proto == "tcp" {
		connString := remoteAddrString + ":" + strconv.Itoa(r.port)

		if conn, err := net.Dial(r.proto, connString); err != nil {
			return err
		} else {
			r.outputPath = conn
		}

	} else {
		return errors.New("Failed to choose a valid proto for " + r.name)
	}

	r.log(r.name + " - Receiver initialized.")

	return nil
}

func (r *Receiver) start() {
	if r.proto == "udp" {

		outFD := r.outputPath.(int) // This file descriptor is for a very raw socket.  The entire packet, including ethernet header, must be constructed.

		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

		srcPort := r.globalConfig.LocalV4Config.Port

		var m *Message

		for m = range r.inputChannel {

			if r.isIPv4 {

				ethernetLayer := &layers.Ethernet{
					SrcMAC:       r.localInterface.HardwareAddr,
					DstMAC:       r.localGatewayHardwareAddr,
					EthernetType: layers.EthernetTypeIPv4,
					Length:       0,
				}

				if r.config.Spoof {
					r.localIp = m.SourceAddress
				}

				ipLayer := &layers.IPv4{
					Version:  4, // IPv4
					TTL:      64,
					Protocol: 17, // UDP
					SrcIP:    r.localIp,
					DstIP:    r.ip,
				}

				if r.config.PreserveUdpPort {
					srcPort = m.SourcePort
				}

				udpLayer := &layers.UDP{
					SrcPort: layers.UDPPort(srcPort),
					DstPort: layers.UDPPort(r.port),
				}

				udpLayer.SetNetworkLayerForChecksum(ipLayer)

				gopacket.SerializeLayers(buf, opts,
					ethernetLayer,
					ipLayer,
					udpLayer,
					gopacket.Payload(m.Payload),
				)

			} else {
				ethernetLayer := &layers.Ethernet{
					SrcMAC:       r.localInterface.HardwareAddr,
					DstMAC:       r.localGatewayHardwareAddr,
					EthernetType: layers.EthernetTypeIPv6,
					Length:       0,
				}

				if r.config.Spoof {
					r.localIp = m.SourceAddress
				}

				ipLayer := &layers.IPv6{
					Version:  6, // IPv6
					HopLimit: 64,
					SrcIP:    r.localIp,
					DstIP:    r.ip,
				}

				if r.config.PreserveUdpPort {
					srcPort = m.SourcePort
				}

				udpLayer := &layers.UDP{
					SrcPort: layers.UDPPort(srcPort),
					DstPort: layers.UDPPort(r.port),
				}

				udpLayer.SetNetworkLayerForChecksum(ipLayer)

				gopacket.SerializeLayers(buf, opts,
					ethernetLayer,
					ipLayer,
					udpLayer,
					gopacket.Payload(m.Payload),
				)
			}

			/*
				When spoofing, care should be taken to only spoof if source and destination are the same IP version,
				but I'll leave that up to the user for now.
			*/

			_, err := syscall.Write(outFD, buf.Bytes())

			if err != nil {
				r.error(err)
			}
		}

		err := syscall.Close(outFD)
		if err != nil {
			r.error(err)
		}
	} else if r.proto == "tcp" {

		conn := r.outputPath.(net.Conn)

		var m *Message

		for m = range r.inputChannel {
			_, err := conn.Write(m.Payload)

			if err != nil {
				r.error(err)
			}
		}

		err := conn.Close()
		if err != nil {
			r.error(err)
		}
	}
}

func (r *Receiver) StartSending(wg *sync.WaitGroup) error {

	err := r.init()

	if err != nil {
		return err
	}

	wg.Add(1)

	go func() {
		r.start()
		r.log(r.name + " - receiver stopped.")
		r.done <- struct{}{}
		wg.Done()
	}()

	return nil
}

func (r *Receiver) Stop() {
	close(r.inputChannel)
	<-r.done
}

func (r *Receiver) AddMessage(msg *Message) error {

	select {
	case r.inputChannel <- msg:
	default:
		return errors.New("Receiver channel is full.")
	}

	return nil
}
