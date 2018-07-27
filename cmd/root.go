package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"nanny/pkg/storage"
	"net/http"
	"os"
	"os/signal"
	"time"

	"nanny/api"
	"nanny/pkg/notifier"

	log "github.com/mgutz/logxi"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config is a config, not much to say here, really.
type Config struct {
	Name       string
	Addr       string
	StorageDSN string `mapstructure:"storage_dsn"`
	Stderr     Stderr
	Email      Email
	Sentry     Sentry
	Twilio     Twilio
	Slack      Slack
}

// Stderr notifier config.
type Stderr struct {
	Enabled bool
}

// Email notifier config.
type Email struct {
	Enabled      bool
	From         string
	To           []string
	Subject      string
	Body         string
	SMTPServer   string `mapstructure:"smtp_server"`
	SMTPPort     int    `mapstructure:"smtp_port"`
	SMTPUser     string `mapstructure:"smtp_user"`
	SMTPPassword string `mapstructure:"smtp_password"`
}

// Sentry notifier config.
type Sentry struct {
	Enabled bool
	DSN     string
}

// Twilio SMS config.
type Twilio struct {
	Enabled    bool
	AccountSID string
	AuthToken  string
	AppSID     string
	From       string
	To         string
}

// Slack config.
type Slack struct {
	Enabled    bool
	WebhookURL string
}

var (
	cfgFile            string // path to configfile
	otherNanny         string // pair nanny that monitors this instance
	otherNannyNotifier string // what notifier to use for otherNanny
	config             Config // parsed config struct
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "nanny",
	Short: "Nanny is a monitor that alerts when your other programs stop breathing",
	Long: `Nanny runs a API, that expects HTTP POST from your program in periodic
intervals. If your program does not call nanny in expected interval, it
notifies you.`,
	Run: run,
}

func run(cmd *cobra.Command, args []string) {
	if otherNanny != "" {
		go nannyCheck(otherNanny)
	}

	runAPI()
}

// nannyCheck runs in a goroutine and sends signal to some other nanny.
func nannyCheck(nanny string) {
	client := http.Client{Timeout: time.Duration(1) * time.Second}
	signal := api.Signal{
		Name:       config.Name,
		Notifier:   otherNannyNotifier,
		NextSignal: "1s",
		Meta:       map[string]string{"addr": config.Addr},
	}
	data, err := json.Marshal(&signal)
	if err != nil {
		log.Error("Unable to marshall JSON to notify pair nanny", "err", err)
		return
	}

	for {
		resp, err := client.Post(nanny, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Error("Unable to notify my pair nanny", "other_nanny", nanny, "err", err)
			time.Sleep(time.Duration(5) * time.Second)
			continue
		}
		if resp.StatusCode != 200 {
			log.Error("Pair nanny returned error", "status_code", resp.StatusCode)
		}

		// We have to sleep for less than 1s, because there will be some network
		// latency added to the request, even on localhost.
		time.Sleep(time.Duration(900) * time.Millisecond)
	}
}

func runAPI() {
	// Create notifiers according to config.
	notifiers, err := makeNotifiers()
	if err != nil {
		log.Fatal("Unable to initialize notifiers", "err", err)
	}
	store, err := storage.NewSQLiteDB(config.StorageDSN)
	if err != nil {
		log.Fatal("Unable to create/load sqlite storage", "dsn", config.StorageDSN, "err", err)
	}
	defer func() {
		err := store.Close()
		if err != nil {
			log.Fatal("Unable to close sqlite storage.")
		}
	}()
	api := api.Server{
		Name:      config.Name,
		Notifiers: notifiers,
		Storage:   store,
	}
	handler, err := api.Handler()
	if err != nil {
		log.Fatal("Unable to create API handlers.", "err", err)
	}

	server := http.Server{
		Addr:    config.Addr,
		Handler: handler,

		// Considering request/response sizes used with Nanny, these values
		// must be plenty.
		ReadTimeout:       time.Duration(10) * time.Second,
		ReadHeaderTimeout: time.Duration(10) * time.Second,
		WriteTimeout:      time.Duration(10) * time.Second,
		IdleTimeout:       time.Duration(10) * time.Second,
	}

	// CTRL+C handling.
	idleConnsClosed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		log.Info("Nanny shutting down.")

		// We received an interrupt signal, shut down.
		if err := server.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout:
			log.Error("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	log.Info("Nanny listening", "addr", server.Addr)
	err = server.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Fatal("Unable to start API server", "err", err)
	}

	<-idleConnsClosed
}

func makeNotifiers() (map[string]notifier.Notifier, error) {
	notifiers := make(map[string]notifier.Notifier)
	if config.Stderr.Enabled {
		notifiers["stderr"] = &notifier.StdErr{}
	}
	if config.Email.Enabled {
		notifiers["email"] = &notifier.Email{
			From:     config.Email.From,
			To:       config.Email.To,
			Subject:  config.Email.Subject,
			Body:     config.Email.Body,
			Server:   config.Email.SMTPServer,
			Port:     config.Email.SMTPPort,
			User:     config.Email.SMTPUser,
			Password: config.Email.SMTPPassword,
		}
	}
	if config.Sentry.Enabled {
		sentryNotifier, err := notifier.NewSentry(config.Sentry.DSN)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create sentry notifier")
		}
		notifiers["sentry"] = sentryNotifier
	}
	if config.Twilio.Enabled {
		notifiers["twilio"] = notifier.NewTwilio(
			config.Twilio.AccountSID,
			config.Twilio.AuthToken,
			config.Twilio.AppSID,
			config.Twilio.From,
			config.Twilio.To,
		)
	}
	if config.Slack.Enabled {
		slackNotifier, err := notifier.NewSlack(config.Slack.WebhookURL)
		if err != nil {
			return nil, errors.Wrap(err, "unable to create slack notifier")
		}
		notifiers["slack"] = slackNotifier
	}

	return notifiers, nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is nanny.toml)")
	RootCmd.PersistentFlags().StringVar(&otherNanny, "nanny", "", "pair with another nanny to monitor this nanny")
	RootCmd.PersistentFlags().StringVar(&otherNannyNotifier, "nanny-notifier", "stderr", "what notifier to use with other nanny")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		// Search config in current working directory with name "nanny" (without extension).
		viper.AddConfigPath(wd)
		viper.SetConfigName("nanny")
	}

	viper.SetEnvPrefix("nanny") // prefix ENV variables with NANNY_
	viper.AutomaticEnv()        // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		err = viper.Unmarshal(&config)
		if err != nil {
			log.Error("Unable to decode config file", "err", err)
		}
		log.Info("Using config file", "path", viper.ConfigFileUsed())
	} else {
		log.Warn("Config not found, using default stderr notifier.")
		config.Stderr.Enabled = true
	}
}
