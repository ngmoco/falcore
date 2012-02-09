package main

import (
	"falcore"
	"flag"
	"fmt"
	"net/http"
	"os"
	"exp/signal"
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
	fmt.Printf("Forking now with socket: %v\n", srv.SocketFd())
	mypath := os.Args[0]
	args := []string{mypath, "-socket", fmt.Sprintf("%v", srv.SocketFd())}
	attr := new(syscall.ProcAttr)
	attr.Files = append([]int(nil), 0, 1, 2, srv.SocketFd())
	pid, err = syscall.ForkExec(mypath, args, attr)
	return
}

// Handle lifecycle events
func handleSignals(srv *falcore.Server) {
	var sig os.Signal
	pid := syscall.Getpid()
	for {
		sig = <-signal.Incoming
		if usig, ok := sig.(os.UnixSignal); ok {
			switch usig {
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
}
