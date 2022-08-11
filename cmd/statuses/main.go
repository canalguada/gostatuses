package main

import (
	"fmt"
	"os"
	"flag"
	"golang.org/x/sys/unix"
	"context"
	"os/signal"
	"time"

	"github.com/canalguada/goprocfs/procmon"

	// Profiling
	// _ "net/http/pprof"
	// "log"
	// "net/http"
)

type Content = procmon.Content
type Status = procmon.Status
type Resource = procmon.Resource

var (
	NewUnit = procmon.NewUnit
	NewResource = procmon.NewResource
	NewService = procmon.NewService
	GetWaitGroup = procmon.GetWaitGroup
)

var (
	resources map[string]*bool
	paths map[string]string
	verbose = flag.Bool("verbose", false, "verbose")
	debug = flag.Bool("debug", false, "debug")
)

func init() {
	resources = map[string]*bool{
		"stat": flag.Bool("stat", false, "stat"),
		"loadavg": flag.Bool("loadavg", false, "loadavg"),
		"cpuinfo": flag.Bool("cpuinfo", false, "cpuinfo"),
		"meminfo": flag.Bool("meminfo", false, "meminfo"),
		"netdev": flag.Bool("netdev", false, "netdev"),
		"all": flag.Bool("all", false, "all statuses"),
	}
	paths = map[string]string{
		"stat": "/proc/stat",
		"loadavg": "/proc/loadavg",
		"cpuinfo": "/proc/cpuinfo",
		"meminfo": "/proc/meminfo",
		"netdev": "/proc/net/dev",
	}
}

func main() {
	flag.Parse()
	if *debug {
		fmt.Println("Debug required.")
	}
	if *(resources["all"]) {
		for k := range resources {
			*(resources[k]) = true
		}
	}
	// Profiling
	// go func() {
	//   log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, unix.SIGHUP)
	ticker := time.NewTicker(time.Second)
	defer func() {
		signal.Stop(signalChan)
		ticker.Stop()
		cancel()
	}()
	statuses := make(chan Status, 32)
	// initialize service/object/resources
	service := NewService(
		statuses,
		`com.github.canalguada.gostatuses`,
		`com.github.canalguada.gostatuses`,
		`/com/github/canalguada/gostatuses`,
	)
	var managed []string
	for _, k := range []string{
		"stat",
		"cpuinfo",
		"loadavg",
		"meminfo",
		"netdev",
	} {
		if *(resources[k]) && k != "all" {
			managed = append(managed, paths[k])
		}
	}
	if len(managed) > 0 {
		service.Object.AddFileResource(managed...)
		service.Object.UpdateTimeBased(0)
	}
	// spin up workers
	wg := GetWaitGroup()
	// manage signals
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop:
			for {
				select {
				case s := <-signalChan:
					switch s {
					case unix.SIGHUP:
						fmt.Println("Receive sighup.")
					case os.Interrupt:
						fmt.Println("Interrupted by user.")
						cancel()
						os.Exit(1)
					}
				case <-ctx.Done():
					fmt.Println("Bye.")
					break loop
				}
			}
	}()
	// launch service and update dbus properties
	var pulseClient PulseClient
	var mprisClient MprisClient
	pulseClient = NewPulseClient()
	service.Object.AddSimpleResource(pulseClient.Rc, nil)
	mprisClient = NewMprisClient()
	service.Object.AddSimpleResource(mprisClient.Rc, nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("Starting service on bus name", service.BusName(), "...")
		service.Connect()
		pulseClient.UpdateVolume(service.Object.Channel)
		mprisClient.Connect(service.Conn, service.Object.Channel)
		service.Run(debug)
	}()
	// send statuses
	wg.Add(1)
	go func() {
		defer wg.Done()
		var elapsed int
		loop:
			for {
				select {
				case <-ctx.Done():
					break loop
				case <-ticker.C:
					elapsed++
					go service.Object.UpdateTimeBased(elapsed)
				case <-pulseClient.Channel:
					go pulseClient.UpdateVolume(service.Object.Channel)
				case message := <-mprisClient.Channel:
					go mprisClient.UpdateMpris(message, service.Object.Channel)
				}
			}
		close(statuses)
	}()
	wg.Wait() // wait on the workers to finish
	service.Object.Close()
	pulseClient.Close()
	mprisClient.Close()
}

// vim: set ft=go fdm=indent ts=2 sw=2 tw=79 noet:
