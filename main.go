package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

const (
	nameFlag      = "name"
	idFlag        = "id"
	hostFlag      = "host"
	portFlag      = "port"
	checkHostFlag = "check-host"
)

func flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: nameFlag, Value: "example"},
		&cli.StringFlag{Name: idFlag, Value: "example-id"},
		&cli.StringFlag{Name: hostFlag, Value: "0.0.0.0"},
		&cli.IntFlag{Name: portFlag, Value: 8000},
		&cli.StringFlag{Name: checkHostFlag, Value: "host.docker.internal"},
	}
}

func main() {
	app := cli.NewApp()
	app.Action = action
	app.Flags = flags()

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func action(ctx *cli.Context) (err error) {
	// create the consul client
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		return errors.Wrap(err, "cannot create new client")
	}

	// register service and service checks
	err = registerService(ctx, client)
	if err != nil {
		return err
	}

	// unregister service and check by service id
	defer func() {
		_ = client.Agent().ServiceDeregister(ctx.String(idFlag))
	}()

	// graceful shutdown
	notifyCtx, cancel := signal.NotifyContext(ctx.Context, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	eg, egx := errgroup.WithContext(notifyCtx)
	eg.Go(func() error {
		for {
			select {
			case <-egx.Done():
				return nil
			case <-time.After(time.Second):
				// print service statuses every second.
				err := printInstStatuses(client, ctx.String(nameFlag))
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
			Addr:    fmt.Sprintf("%s:%d", ctx.String(hostFlag), ctx.Int(portFlag)),
			Handler: mux,
		}

		go func() {
			<-egx.Done()
			_ = srv.Shutdown(context.Background())
		}()

		log.Printf("listen on %s:%d", ctx.String(hostFlag), ctx.Int(portFlag))

		return srv.ListenAndServe()
	})

	log.Printf("service_id=%s is running", ctx.String(idFlag))

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

func registerService(ctx *cli.Context, client *capi.Client) error {
	id := ctx.String(idFlag)
	name := ctx.String(nameFlag)
	host := ctx.String(hostFlag)
	port := ctx.Int(portFlag)

	fmt.Println(id, name, host, port)

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	err = client.Agent().ServiceRegister(&capi.AgentServiceRegistration{
		Kind:    capi.ServiceKindTypical,
		ID:      id,
		Name:    name,
		Tags:    []string{"sd"},
		Port:    port,
		Address: hostname,
		Check: &capi.AgentServiceCheck{
			CheckID:  "check-" + id,
			Name:     "example-check",
			Interval: "3s",
			Timeout:  "1s",
			HTTP:     fmt.Sprintf("http://%s:%d/healthcheck", ctx.String(checkHostFlag), ctx.Int(portFlag)),
		},
	})
	if err != nil {
		return errors.Wrap(err, "cannot service register")
	}

	return nil
}
