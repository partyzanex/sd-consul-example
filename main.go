package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
	name      = flag.String("name", "example", "service name")
	id        = flag.String("id", "example-id", "service id")
	host      = flag.String("host", "0.0.0.0", "HTTP service hostname")
	port      = flag.Int("port", 8000, "HTTP service port")
	checkHost = flag.String("check-host", "host.docker.internal", "internal hostname for service checks")
)

func main() {
	flag.Parse()

	if err := action(); err != nil {
		log.Fatal(err)
	}
}

func action() (err error) {
	// create the consul client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		return errors.Wrap(err, "cannot create new client")
	}

	// register service and service checks
	err = registerService(client)
	if err != nil {
		return err
	}

	// unregister service and check by service id
	defer func() {
		//_ = client.Agent().ServiceDeregister(*id)
	}()

	// graceful shutdown
	notifyCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	eg, egx := errgroup.WithContext(notifyCtx)
	eg.Go(func() error {
		for {
			select {
			case <-egx.Done():
				return nil
			case <-time.After(time.Second):
				// print service statuses every second.
				err := printInstStatuses(client, *name)
				if err != nil {
					return err
				}
			}
		}
	})
	eg.Go(func() error {
		// HTTP server with /healthcheck
		mux := http.NewServeMux()
		mux.HandleFunc("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		srv := http.Server{
			Addr:    fmt.Sprintf("%s:%d", *host, *port),
			Handler: mux,
		}

		go func() {
			<-egx.Done()
			_ = srv.Shutdown(context.Background())
		}()

		log.Printf("listen on %s:%d", *host, *port)

		return srv.ListenAndServe()
	})

	log.Printf("service_id=%s is running", *id)

	return eg.Wait()
}

func printInstStatuses(client *capi.Client, name string) error {
	_, checks, err := client.Agent().AgentHealthServiceByName(name)
	if err != nil {
		return err
	}

	for _, check := range checks {
		log.Printf(
			"service %s, status %s, address %s:%d",
			check.Service.ID, check.AggregatedStatus,
			check.Service.Address, check.Service.Port,
		)
	}

	return nil
}

func registerService(client *capi.Client) error {
	hostname, err := os.Hostname()
	if err != nil {
		return errors.Wrap(err, "cannot get hostname")
	}

	err = client.Agent().ServiceRegister(&capi.AgentServiceRegistration{
		Kind:    capi.ServiceKindTypical,
		ID:      *id,
		Name:    *name,
		Tags:    []string{"sd"},
		Port:    *port,
		Address: hostname,
		Check: &capi.AgentServiceCheck{
			CheckID:  "check-" + *id,
			Name:     *name + "-check",
			Interval: "3s",
			Timeout:  "1s",
			HTTP:     fmt.Sprintf("http://%s:%d/healthcheck", *checkHost, *port),
		},
	})
	if err != nil {
		return errors.Wrap(err, "cannot service register")
	}

	return nil
}
