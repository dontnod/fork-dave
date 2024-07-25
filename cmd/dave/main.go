package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	syslog "log"
	"net/http"

	"github.com/micromata/dave/app"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/webdav"
)

func main() {
	var configPath string

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.Parse()

	config := app.ParseConfig(configPath)

	// Set formatter for logrus
	formatter := &log.TextFormatter{}
	log.SetFormatter(formatter)

	// Set formatter for default log outputs
	logger := log.New()
	logger.Formatter = formatter
	writer := logger.Writer()
	defer writer.Close()
	syslog.SetOutput(writer)

	wdHandler := &webdav.Handler{
		Prefix: config.Prefix,
		FileSystem: &app.Dir{
			Config: config,
		},
		LockSystem: webdav.NewMemLS(),
		Logger: func(request *http.Request, err error) {
			if config.Log.Error && err != nil {
				log.Error(err)
			}
		},
	}

	a := &app.App{
		Config:  config,
		Handler: wdHandler,
	}

	var tlsConfig *tls.Config
	if config.TLS != nil {
		var secureCiphers []uint16
		for _, cipherSuite := range tls.CipherSuites() {
			if !cipherSuite.Insecure {
				secureCiphers = append(secureCiphers, cipherSuite.ID)
			}
		}
		tlsConfig = &tls.Config{
			CipherSuites: secureCiphers,
			MinVersion:   tls.VersionTLS12,
		}
	}

	http.Handle("/", wrapRecovery(app.NewBasicAuthWebdavHandler(a), config))
	connAddr := fmt.Sprintf("%s:%s", config.Address, config.Port)
	server := http.Server{
		Addr:      connAddr,
		TLSConfig: tlsConfig,
	}

	if config.TLS != nil {
		log.WithFields(log.Fields{
			"address":  config.Address,
			"port":     config.Port,
			"security": "TLS",
		}).Info("Server is starting and listening")
		log.Fatal(server.ListenAndServeTLS(config.TLS.CertFile, config.TLS.KeyFile))

	} else {
		log.WithFields(log.Fields{
			"address":  config.Address,
			"port":     config.Port,
			"security": "none",
		}).Info("Server is starting and listening")
		log.Fatal(server.ListenAndServe())
	}
}

func wrapRecovery(handler http.Handler, config *app.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				switch t := err.(type) {
				case string:
					log.WithError(errors.New(t)).Error("An error occurred handling a webdav request")
				case error:
					log.WithError(t).Error("An error occurred handling a webdav request")
				}
			}
		}()

		if len(config.Cors.Origin) > 0 {
			w.Header().Set("Access-Control-Allow-Origin", config.Cors.Origin)
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Methods", "*")
			if config.Cors.Credentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		handler.ServeHTTP(w, r)
	})
}
