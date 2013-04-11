package main

import (
	"flag"
	"fmt"
	"github.com/ngmoco/falcore"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

// very simple request filter
func Filter(request *falcore.Request) *http.Response {
	return falcore.SimpleResponse(request.HttpRequest, 200, nil, "OK\n")
}

// flag to accept a socket file descriptor
var socketFd = flag.Int("socket", -1, "Socket file descriptor")

func main() {
	pid := syscall.Getpid()
	flag.Parse()

	// create the pipeline
	pipeline := falcore.NewPipeline()
	pipeline.Upstream.PushBack(falcore.NewRequestFilter(Filter))

	// create the server with the pipeline
	srv := falcore.NewServer(8090, pipeline)

	// if passed the socket file descriptor, setup the listener that way
	// if you don't have it, the default is to create the socket listener
	// with the data passed to falcore.NewServer above (happens in ListenAndServer())
	if *socketFd != -1 {
		// I know I'm a child process if I get here so I can signal the parent when I'm ready to take over
		go childReady(srv)
		fmt.Printf("%v Got socket FD: %v\n", pid, *socketFd)
		srv.FdListen(*socketFd)
	}

	// using signals to manage the restart lifecycle
	go handleSignals(srv)

	// start the server
	// this is normally blocking forever unless you send lifecycle commands 
	fmt.Printf("%v Starting Listener on 8090\n", pid)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Printf("%v Could not start server: %v", pid, err)
	}
	fmt.Printf("%v Exiting now\n", pid)
}

// blocks on the server ready and when ready, it sends 
// a signal to the parent so that it knows it cna now exit
func childReady(srv *falcore.Server) {
	pid := syscall.Getpid()
	// wait for the ready signal
	<-srv.AcceptReady
	// grab the parent and send a signal that the child is ready
	parent := syscall.Getppid()
	fmt.Printf("%v Kill parent %v with SIGUSR1\n", pid, parent)
	syscall.Kill(parent, syscall.SIGUSR1)
}

// setup and fork/exec myself. Make sure to keep open important FD's that won't get re-created by the child
// specifically, std* and your listen socket
func forker(srv *falcore.Server) (pid int, err error) {
	var socket string
	// At version 1.0.3 the socket FD behavior changed and the fork socket is always 3
	// 0 = stdin, 1 = stdout, 2 = stderr, 3 = acceptor socket
	// This is because the ForkExec dups all the saved FDs down to
	// start at 0.  This is also why you MUST include 0,1,2 in the
	// attr.Files
	if goVersion103OrAbove() {
		socket = "3"
	} else {
		socket = fmt.Sprintf("%v", srv.SocketFd())
	}
	fmt.Printf("Forking now with socket: %v\n", socket)
	mypath := os.Args[0]
	args := []string{mypath, "-socket", socket}
	attr := new(syscall.ProcAttr)
	attr.Files = append([]uintptr(nil), 0, 1, 2, uintptr(srv.SocketFd()))
	pid, err = syscall.ForkExec(mypath, args, attr)
	return
}

func goVersion103OrAbove() bool {
	ver := strings.Split(runtime.Version(), ".")
	// Go versioning is weird so this only works for common go1 cases:
	// current as of patch:
	// go1.0.3                        13678:2d8bc3c94ecb : true
	// go1.0.2                        13278:5e806355a9e1 : false
	// go1.0.1                        12994:2ccfd4b451d3 : false
	// go1                            12872:920e9d1ffd1f : false
	// go1.1+/go2+ : true
	// release* : true (this is possibly broken)
	// weekly* : true (this is possibly broken)
	// tip : true
	if len(ver) > 0 && strings.Index(ver[0], "go") == 0 {
		if ver[0] == "go1" && len(ver) == 1 {
			// just go1
			return false
		} else if ver[0] == "go1" && len(ver) == 3 && ver[1] == "0" {
			if patchVer, _ := strconv.ParseInt(ver[2], 10, 64); patchVer < 3 {
				return false
			}
		}
	}
	return true
}

// Handle lifecycle events
func handleSignals(srv *falcore.Server) {
	var sig os.Signal
	var sigChan = make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGUSR1, syscall.SIGINT, syscall.SIGTERM, syscall.SIGTSTP)
	pid := syscall.Getpid()
	for {
		sig = <-sigChan
		switch sig {
		case syscall.SIGHUP:
			// send this to the paraent process to initiate the restart
			fmt.Println(pid, "Received SIGHUP.  forking.")
			cpid, err := forker(srv)
			fmt.Println(pid, "Forked pid:", cpid, "errno:", err)
		case syscall.SIGUSR1:
			// child sends this back to the parent when it's ready to Accept
			fmt.Println(pid, "Received SIGUSR1.  Stopping accept.")
			srv.StopAccepting()
		case syscall.SIGINT:
			fmt.Println(pid, "Received SIGINT.  Shutting down.")
			os.Exit(0)
		case syscall.SIGTERM:
			fmt.Println(pid, "Received SIGTERM.  Terminating.")
			os.Exit(0)
		case syscall.SIGTSTP:
			fmt.Println(pid, "Received SIGTSTP.  Stopping.")
			syscall.Kill(pid, syscall.SIGSTOP)
		default:
			fmt.Println(pid, "Received", sig, ": ignoring")
		}
	}
}
