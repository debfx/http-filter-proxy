// SPDX-License-Identifier: GPL-2.0-only OR GPL-3.0-only
// Copyright (C) 2020-2022 Felix Geyer <debfx@fobos.de>

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	systemdDaemon "github.com/coreos/go-systemd/v22/daemon"
	"github.com/elazarl/goproxy"
	"github.com/gobwas/glob"
	flag "github.com/spf13/pflag"
)

var allowedHostGlobs []glob.Glob

func isHostAllowed(hostname string) bool {
	hostnameWithoutPort := strings.Split(hostname, ":")[0]

	for _, g := range allowedHostGlobs {
		if g.Match(hostnameWithoutPort) {
			return true
		}
	}

	return false
}

func main() {
	allowedHostnames := []string{}
	listenTCP := flag.String("listen", ":8080", "Listen on tcp socket (example: \":8080\" or \"127.0.0.1:9999\").")
	flag.StringSliceVar(&allowedHostnames, "allow", []string{}, "Allow connections to this hostname.")
	verbose := flag.Bool("verbose", false, "Verbose logging.")

	flag.Parse()

	for _, hostname := range allowedHostnames {
		allowedHostGlobs = append(allowedHostGlobs, glob.MustCompile(hostname, '.'))
	}

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = *verbose

	proxy.OnRequest().DoFunc(
		func(request *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			if isHostAllowed(request.Host) {
				if *verbose {
					fmt.Printf("HTTP ALLOWED %s\n", request.Host)
				}
				return request, nil
			}

			fmt.Printf("HTTP REJECTED %s\n", request.Host)
			return request, goproxy.NewResponse(request, goproxy.ContentTypeText, http.StatusForbidden, "Forbidden")
		})

	proxy.OnRequest().HandleConnectFunc(
		func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
			if isHostAllowed(host) {
				if *verbose {
					fmt.Printf("CONNECT ALLOWED %s\n", host)
				}
				return goproxy.OkConnect, host
			}

			fmt.Printf("CONNECT REJECTED %s\n", host)
			return goproxy.RejectConnect, host
		})

	httpServer := &http.Server{Handler: proxy}

	tcpListener, err := net.Listen("tcp", *listenTCP)
	if err != nil {
		fmt.Printf("failed to listen on port %s: %v\n", *listenTCP, err)
		os.Exit(1)
	}
	defer tcpListener.Close()

	_, err = systemdDaemon.SdNotify(false, systemdDaemon.SdNotifyReady)
	if err != nil {
		fmt.Printf("failed to notify init daemon: %v\n", err)
	}

	go func() {
		done := make(chan os.Signal, 1)
		signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
		<-done

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = httpServer.Shutdown(ctx)
		if err != nil {
			fmt.Printf("failed to shutdown http proxy: %v\n", err)
		}
	}()

	err = httpServer.Serve(tcpListener)
	if err != nil && err != http.ErrServerClosed {
		fmt.Printf("failed to serve http proxy: %v\n", err)
		os.Exit(1)
	}
}
