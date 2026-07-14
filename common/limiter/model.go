package limiter

type GlobalDeviceLimitConfig struct {
	Enable        bool   `mapstructure:"Enable" json:"Enable" yaml:"Enable"`
	RedisNetwork  string `mapstructure:"RedisNetwork" json:"RedisNetwork" yaml:"RedisNetwork"` // tcp or unix
	RedisAddr     string `mapstructure:"RedisAddr" json:"RedisAddr" yaml:"RedisAddr"`          // host:port, or /path/to/unix.sock
	RedisUsername string `mapstructure:"RedisUsername" json:"RedisUsername" yaml:"RedisUsername"`
	RedisPassword string `mapstructure:"RedisPassword" json:"RedisPassword" yaml:"RedisPassword"`
	RedisDB       int    `mapstructure:"RedisDB" json:"RedisDB" yaml:"RedisDB"`
	Timeout       int    `mapstructure:"Timeout" json:"Timeout" yaml:"Timeout"`
	Expiry        int    `mapstructure:"Expiry" json:"Expiry" yaml:"Expiry"` // second
}
