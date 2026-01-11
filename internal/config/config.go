package config

type RemoteConfig struct {
	Name               string `mapstructure:"name"`
	Path               string `mapstructure:"path"`
	Endpoint           string `mapstructure:"endpoint"`
	Key                string `mapstructure:"key"`
	StabilityThreshold int    `mapstructure:"stability_threshold"` // Checks in worker
	CheckInterval      string `mapstructure:"check_interval"`      // Time between worker checks
	StabilityTimeout   string `mapstructure:"stability_timeout"`   // Max wait time
	ConcurrencyLimit   int    `mapstructure:"concurrency_limit"`   // Max parallel uploads
	PollingInterval    string `mapstructure:"polling_interval"`    // Backup scan frequency
	SettlingDelay      string `mapstructure:"settling_delay"`      // Initial "quiet" period
	DisableFsnotify    bool   `mapstructure:"disable_fsnotify"`    // Disable real-time watcher
}
