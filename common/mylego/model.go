package mylego

type CertConfig struct {
	CertMode         string            `mapstructure:"CertMode" json:"CertMode" yaml:"CertMode"` // none, file, http, dns
	CertDomain       string            `mapstructure:"CertDomain" json:"CertDomain" yaml:"CertDomain"`
	CertFile         string            `mapstructure:"CertFile" json:"CertFile" yaml:"CertFile"`
	KeyFile          string            `mapstructure:"KeyFile" json:"KeyFile" yaml:"KeyFile"`
	Provider         string            `mapstructure:"Provider" json:"Provider" yaml:"Provider"`
	Email            string            `mapstructure:"Email" json:"Email" yaml:"Email"`
	DNSEnv           map[string]string `mapstructure:"DNSEnv" json:"DNSEnv,omitempty" yaml:"DNSEnv,omitempty"`
	RejectUnknownSni bool              `mapstructure:"RejectUnknownSni" json:"RejectUnknownSni" yaml:"RejectUnknownSni"`
}

type LegoCMD struct {
	C    *CertConfig
	path string
}
