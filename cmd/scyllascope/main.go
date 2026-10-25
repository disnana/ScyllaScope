package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"scyllascope/internal/alerts"
	"scyllascope/internal/api"
	"scyllascope/internal/config"
	"scyllascope/internal/doctor"
	"scyllascope/internal/model"
	"scyllascope/internal/monitor"
	"scyllascope/internal/scyllaclient"
	"scyllascope/internal/service"
	"scyllascope/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			initConfig(os.Args[2:])
			return
		case "service":
			serviceCmd(os.Args[2:])
			return
		case "detect":
			detectCmd()
			return
		case "doctor":
			doctorCmd(os.Args[2:])
			return
		case "version":
			fmt.Println("ScyllaScope " + model.Version)
			return
		case "config":
			configCmd(os.Args[2:])
			return
		}
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fs := flag.NewFlagSet("scyllascope", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/scyllascope/config.yaml", "optional config file; missing values are auto-detected from Scylla")
	_ = fs.Parse(os.Args[1:])
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	log.Printf("config source=%s cluster=%q node=%s listen=%s cql=%v:%d", cfg.Source, cfg.ClusterName, cfg.NodeID, cfg.Listen, cfg.Scylla.CQLHosts, cfg.Scylla.CQLPort)
	st, err := store.Open(cfg.Storage.SQLitePath)
	if err != nil {
		return err
	}
	defer st.Close()

	client, err := scyllaclient.New(cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	tlsCfg, err := scyllaclient.TLS(cfg)
	if err != nil {
		return err
	}
	mon := monitor.New(cfg, st, client, tlsCfg)
	alert := alerts.New(cfg, st)
	mon.AfterCollect = alert.Evaluate
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	monDone := make(chan struct{})
	alertDone := make(chan struct{})
	go func() { defer close(monDone); mon.Run(ctx) }()
	go func() { defer close(alertDone); alert.Run(ctx) }()
	srv := &http.Server{Addr: cfg.Listen, Handler: api.New(cfg, mon, st, alert).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("ScyllaScope %s listening=%s auth=%t; TCP success is not replica consistency", model.Version, cfg.Listen, cfg.Security.UIToken != "")
		if cfg.Security.UIToken == "" {
			log.Printf("UI token not configured: read-only mode; use service install to generate a login token")
		}
		if cfg.Security.TLSCert != "" {
			errCh <- srv.ListenAndServeTLS(cfg.Security.TLSCert, cfg.Security.TLSKey)
		} else {
			errCh <- srv.ListenAndServe()
		}
	}()
	var serverErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			serverErr = err
		}
		cancel()
	}
	sd, cancelSD := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelSD()
	if err := srv.Shutdown(sd); err != nil {
		log.Printf("HTTP shutdown: %v; closing remaining connections", err)
		_ = srv.Close()
	}
	select {
	case <-monDone:
	case <-sd.Done():
		log.Printf("monitor shutdown deadline reached")
	}
	select {
	case <-alertDone:
	case <-sd.Done():
	}
	return serverErr
}

func initConfig(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	out := fs.String("output", "./scyllascope.yaml", "output config path")
	_ = fs.Parse(args)
	cfg, err := config.Detect()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(*out); err == nil {
		log.Fatal("output already exists; choose a new file to avoid overwriting settings")
	}
	if err := config.WriteOverrideTemplate(*out, cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Detected Scylla settings and wrote override template %s\n", *out)
	fmt.Printf("cluster=%q node=%s listen=%s CQL=%v:%d internode=%d\n", cfg.ClusterName, cfg.NodeID, cfg.Listen, cfg.Scylla.CQLHosts, cfg.Scylla.CQLPort, cfg.Scylla.InternodePort)
}

func detectCmd() {
	cfg, err := config.Detect()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("cluster: %s\nnode: %s (%s)\nui: http://%s\ncql: %v:%d\ninternode: %d\nrest: %s\nscylla_config: %s\n", cfg.ClusterName, cfg.NodeName, cfg.NodeID, cfg.Listen, cfg.Scylla.CQLHosts, cfg.Scylla.CQLPort, cfg.Scylla.InternodePort, cfg.Scylla.RESTURL, cfg.Scylla.ConfigPath)
}

// Reset is intentionally not automatic: an override can contain credentials,
// peer keys, notification destinations, and security policy.
func configCmd(args []string) {
	fmt.Fprintln(os.Stderr, "config reset is disabled to protect credentials. Edit /etc/scyllascope/config.yaml, removing only the auto-detected keys you no longer want to override. `init --output NEW.yaml` writes a new template; it never resets the installed file.")
	os.Exit(2)
}

func doctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	cfgPath := fs.String("config", "/etc/scyllascope/config.yaml", "optional config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if !doctor.Print(doctor.Run(cfg)) {
		os.Exit(1)
	}
}

func serviceCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: scyllascope service <install|uninstall|status|restart|logs|rollback>")
		os.Exit(2)
	}
	switch args[0] {
	case "install":
		fs := flag.NewFlagSet("service install", flag.ExitOnError)
		cfgPath := fs.String("config", "/etc/scyllascope/config.yaml", "config file to read/merge and install")
		_ = fs.Parse(args[1:])
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := service.Install(*cfgPath, cfg); err != nil {
			log.Fatal(err)
		}
		fmt.Println("ScyllaScope installed and started. Use `scyllascope service status`.")
	case "uninstall":
		fs := flag.NewFlagSet("service uninstall", flag.ExitOnError)
		purge := fs.Bool("purge", false, "also remove /etc/scyllascope and /var/lib/scyllascope")
		_ = fs.Parse(args[1:])
		if err := service.Uninstall(*purge); err != nil {
			log.Fatal(err)
		}
		fmt.Println("ScyllaScope system service removed.")
	case "rollback":
		if err := service.Rollback(); err != nil {
			log.Fatal(err)
		}
	case "status":
		if err := service.Status(); err != nil {
			os.Exit(1)
		}
	case "restart":
		if err := service.Restart(); err != nil {
			log.Fatal(err)
		}
	case "logs":
		if err := service.Logs(); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: scyllascope service <install|uninstall|status|restart|logs|rollback>")
		os.Exit(2)
	}
}
