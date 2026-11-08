// Command netsurf is the interactive browser shell.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"gonetsurf/rendering/backends"
	"gonetsurf/runtime/sandbox"
	"gonetsurf/shells"
	"gonetsurf/urls"
	"gonetsurf/windowing/x11"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--broker" {
		if err := runBroker(); err != nil {
			fmt.Fprintln(os.Stderr, "netsurf broker:", err)
			os.Exit(1)
		}
		return
	}
	rendererName, arguments, err := parseRendererArguments(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurf:", err)
		os.Exit(2)
	}
	var initial *urls.URL
	if len(arguments) > 0 {
		parsed, parseErr := urls.Parse(arguments[0])
		if parseErr != nil {
			if candidate, candidateErr := urls.Parse("https://" + arguments[0]); candidateErr == nil {
				parsed = candidate
			} else {
				fmt.Fprintln(os.Stderr, "netsurf:", parseErr)
				os.Exit(2)
			}
		}
		initial = parsed
	}
	backend, err := backends.Select(rendererName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurf:", err)
		os.Exit(2)
	}
	renderer, err := backend.NewRenderer()
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurf:", err)
		os.Exit(1)
	}
	defer renderer.Close()

	broker, err := startBroker()
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurf:", err)
		os.Exit(1)
	}
	defer broker.stop()

	const width = 1024
	const height = 768
	shell := shells.New(width, height, nil, shells.BrokerClients{Network: broker.network, Storage: broker.storage})
	shell.SetRenderer(renderer)
	if initial == nil {
		_ = shell.LoadSession(context.Background())
	}
	if initial != nil {
		shell.NavigateInitial(initial)
	}
	display := x11.New(x11.Options{Width: width, Height: height, Title: "NetSurf"})
	opened, err := display.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "netsurf:", err)
		os.Exit(1)
	}
	defer display.Close()
	if err := sandbox.ApplyUnprivilegedBaseline(); err != nil {
		fmt.Fprintln(os.Stderr, "netsurf: sandbox:", err)
		os.Exit(1)
	}
	if err := sandbox.InstallNetworkSocketGuard(); err != nil {
		fmt.Fprintln(os.Stderr, "netsurf: network sandbox:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loopDone := make(chan error, 1)
	go func() {
		loopErr := display.EventLoop(ctx)
		if loopErr != nil {
			cancel()
		}
		loopDone <- loopErr
	}()

	runErr := shell.Run(ctx, opened)
	_ = shell.SaveSession(context.Background())
	cancel()
	select {
	case loopErr := <-loopDone:
		if runErr == nil && loopErr != nil && !errors.Is(loopErr, context.Canceled) {
			runErr = loopErr
		}
	default:
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		fmt.Fprintln(os.Stderr, "netsurf:", runErr)
		os.Exit(1)
	}
}

func parseRendererArguments(arguments []string) (string, []string, error) {
	name := "auto"
	remaining := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--renderer" {
			if index+1 >= len(arguments) {
				return "", nil, fmt.Errorf("--renderer requires a value")
			}
			name = arguments[index+1]
			index++
			continue
		}
		if strings.HasPrefix(argument, "--renderer=") {
			name = strings.TrimPrefix(argument, "--renderer=")
			continue
		}
		remaining = append(remaining, argument)
	}
	return name, remaining, nil
}
