package config

type LocalConfig struct {
	Interface  string `yaml:"interface"`
	Address    string `yaml:"address"`
	GatewayMAC string `yaml:"gateway_mac"`
	Port       int    `yaml:"port"`
}

type SourceConfig struct {
	Name      string   `yaml:"name"`
	SourceIP  string   `yaml:"source_ip"`
	Receivers []string `yaml:"receivers"`
}

type ReceiverConfig struct {
	Name    string `yaml:"name"`
	Spoof   bool   `yaml:"spoof"`
	Proto   string `yaml:"proto"`
	Ip      string `yaml:"ip"`
	IPvPref int    `yaml:"ipv_pref"`
	Port    int    `yaml:"port"`
}

type Config struct {
	LocalV4Config LocalConfig      `yaml:"local_v4"`
	LocalV6Config LocalConfig      `yaml:"local_v6"`
	Sources       []SourceConfig   `yaml:"sources"`
	Receivers     []ReceiverConfig `yaml:"receivers"`
}
