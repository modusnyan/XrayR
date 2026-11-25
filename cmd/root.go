package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/XrayR-project/XrayR/config"
	"github.com/XrayR-project/XrayR/observability"
	"github.com/XrayR-project/XrayR/panel"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:           "XrayR",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "Config file for XrayR.")
}

func run() error {
	showVersion()

	configPath := resolveConfigPath()
	result, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if result.HasErrors() {
		_ = printIssues(os.Stderr, "text", result)
		return fmt.Errorf("configuration is invalid")
	}
	panelConfig := result.Config
	configureLogger(panelConfig.LogConfig)
	setConfigEnvironment(configPath)

	p := panel.New(panelConfig)
	if err := p.Start(); err != nil {
		return err
	}
	lastTime := time.Now()

	watcher := viper.New()
	watcher.SetConfigFile(configPath)
	if err := watcher.ReadInConfig(); err != nil {
		return err
	}
	watcher.WatchConfig()
	watcher.OnConfigChange(func(e fsnotify.Event) {
		if !time.Now().After(lastTime.Add(3 * time.Second)) {
			return
		}
		// Record the attempt even when the new configuration is rejected, so a
		// burst of invalid writes does not re-run validation on every event.
		lastTime = time.Now()
		newResult, loadErr := config.Load(configPath)
		if loadErr != nil || newResult.HasErrors() {
			log.WithError(loadErr).Error("New configuration is invalid; continuing with the previous configuration")
			if loadErr == nil {
				_ = printIssues(os.Stderr, "text", newResult)
			}
			return
		}

		log.WithField("path", e.Name).Info("Configuration changed, applying validated configuration")
		oldConfig := panelConfig
		if err := p.Close(); err != nil {
			log.WithError(err).Warn("Previous configuration did not close cleanly")
		}
		runtime.GC()
		candidate := panel.New(newResult.Config)
		configureLogger(newResult.Config.LogConfig)
		if err := candidate.Start(); err != nil {
			observability.Reloads.WithLabelValues("rollback").Inc()
			log.WithError(err).Error("New configuration failed to start; rolling back to previous configuration")
			rollback := panel.New(oldConfig)
			if rollbackErr := rollback.Start(); rollbackErr != nil {
				// Neither the new nor the previous configuration is running, so the
				// process would otherwise stay alive while serving nothing. Fail loudly
				// and let the service manager restart us.
				observability.Reloads.WithLabelValues("failed").Inc()
				log.WithError(rollbackErr).Error("Rollback configuration failed to start; no node is serving traffic, exiting")
				os.Exit(1)
			}
			p = rollback
			panelConfig = oldConfig
		} else {
			observability.Reloads.WithLabelValues("success").Inc()
			p = candidate
			panelConfig = newResult.Config
		}
	})

	// Capture the panel by reference: a successful reload replaces p, and the panel
	// that must be closed on shutdown is the one currently running.
	defer func() {
		if err := p.Close(); err != nil {
			log.WithError(err).Warn("Shutdown did not close the panel cleanly")
		}
	}()
	runtime.GC()
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals
	return nil
}

func configureLogger(config *panel.LogConfig) {
	if config == nil {
		return
	}
	if strings.EqualFold(config.Format, "json") {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	}
	if level, err := log.ParseLevel(config.Level); err == nil {
		log.SetLevel(level)
	}
	log.SetReportCaller(strings.EqualFold(config.Level, "debug"))
}

func setConfigEnvironment(configPath string) {
	configPath = path.Dir(configPath)
	_ = os.Setenv("XRAY_LOCATION_ASSET", configPath)
	_ = os.Setenv("XRAY_LOCATION_CONFIG", configPath)
}

func Execute() error {
	return rootCmd.Execute()
}
