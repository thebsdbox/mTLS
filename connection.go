package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func (c *Config) startInternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("%s:%d", c.Address, c.ProxyPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("Failed to start proxy server: %v", err)
	}
	log.Printf("Internal listener [pid: %d] %s", os.Getpid(), proxyAddr)
	return listener
}

func (c *Config) startExternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", c.ClusterPort)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("Failed to start proxy server: %v", err)
	}
	log.Printf("External listener on %s", proxyAddr)
	return listener
}

// Blocking function
func (c *Config) start(listener net.Listener, internal bool) {
	for {
		conn, err := listener.Accept()
		if (err != nil) && !errors.Is(err, net.ErrClosed) {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		if internal {
			log.Printf("internal proxy connection from %s -> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())

			go c.internalProxy(conn)
		} else {
			log.Printf("external proxy connection from %s -> %s", conn.RemoteAddr().String(), conn.LocalAddr().String())

			go c.handleExternalConnection(conn)
		}
	}
}

// HTTP proxy request handler
func (c *Config) internalProxy(conn net.Conn) {
	defer conn.Close()
	// Get original destination address
	destAddr, destPort, err := findTargetFromConnection(conn)
	if err != nil {
		return
	}
	targetDestination := fmt.Sprintf("%s:%d", destAddr, destPort)

	// Send traffic to endpoint gateway
	endpoint := fmt.Sprintf("%s:%d", destAddr, c.ClusterPort)
	if c.ClusterAddress != "" {
		endpoint = fmt.Sprintf("%s:%d", c.ClusterAddress, c.ClusterPort)
	}
	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", endpoint, 5*time.Second)
	if err != nil {
		log.Printf("Failed to connect to original destination: %v", err)
		return
	}
	defer targetConn.Close()

	log.Printf("Connected to remote endpoint %s, original dest %s", endpoint, targetDestination)
	//log.Printf("Internal proxy sending original destination: %s\n", targetDestination)
	targetConn.Write([]byte(targetDestination))
	tmp := make([]byte, 256)

	// Ideally we wait here until our remote endpoint has recieved the targetDestination
	targetConn.Read(tmp)

	//log.Printf("Internal connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	go func() {
		_, err = io.Copy(targetConn, conn)
		if err != nil {
			log.Printf("Failed copying data to target: %v", err)
		}
	}()
	_, err = io.Copy(conn, targetConn)
	if err != nil {
		log.Printf("Failed copying data from target: %v", err)
	}
}

// HTTP proxy request handler
func (c *Config) handleExternalConnection(conn net.Conn) {
	defer conn.Close()
	// targetAddr, targetPort, err := findTargetFromConnection(conn)
	// if err != nil {
	// 	return
	// }
	// fmt.Printf("External: %s:%d\n", targetAddr, targetPort)

	tmp := make([]byte, 256)
	n, err := conn.Read(tmp)
	if err != nil {
		log.Print(err)
	}
	remoteAddress := string(tmp[:n])
	log.Printf("WUT %s", remoteAddress)

	if remoteAddress == fmt.Sprintf("%s:%d", c.Address, c.ProxyPort) {
		log.Printf("Potential loopback")
		return
	}

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", remoteAddress, 5*time.Second)
	if err != nil {
		log.Printf("Failed to connect to original destination[%s]: %v", string(tmp), err)
		return
	}
	defer targetConn.Close()
	conn.Write([]byte{'Y'}) // Send a response to kickstart the comms

	log.Printf("External connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

	// The following code creates two data transfer channels:
	// - From the client to the target server (handled by a separate goroutine).
	// - From the target server to the client (handled by the main goroutine).
	go func() {
		_, err = io.Copy(targetConn, conn)
		if err != nil {
			log.Printf("Failed copying data to target: %v", err)
		}
	}()
	_, err = io.Copy(conn, targetConn)
	if err != nil {
		log.Printf("Failed copying data from target: %v", err)
	}
}
