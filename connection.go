package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func startInternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", PROXY_PORT)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("Failed to start proxy server: %v", err)
	}
	log.Printf("Internal listener [pid: %d] %s", os.Getpid(), proxyAddr)
	return listener
}

func startExternalListener() net.Listener {
	proxyAddr := fmt.Sprintf("0.0.0.0:%d", PROXY_PORT+1)
	listener, err := net.Listen("tcp", proxyAddr)
	if err != nil {
		log.Fatalf("Failed to start proxy server: %v", err)
	}
	log.Printf("External listener on %s", proxyAddr)
	return listener
}

// Blocking function
func start(listener net.Listener, internal bool) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		if internal {
			go handleInternalConnection(conn)
		} else {
			go handleExternalConnection(conn)
		}
	}
}
func findTargetFromConnection(conn net.Conn) (targetAddr string, targetPort uint16, err error) {
	// Using RawConn is necessary to perform low-level operations on the underlying socket file descriptor in Go.
	// This allows us to use getsockopt to retrieve the original destination address set by the SO_ORIGINAL_DST option,
	// which isn't directly accessible through Go's higher-level networking API.
	rawConn, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		log.Printf("Failed to get raw connection: %v", err)
		return
	}

	var originalDst SockAddrIn
	// If Control is not nil, it is called after creating the network connection but before binding it to the operating system.
	rawConn.Control(func(fd uintptr) {
		optlen := uint32(unsafe.Sizeof(originalDst))
		// Retrieve the original destination address by making a syscall with the SO_ORIGINAL_DST option.
		err = getsockopt(int(fd), syscall.SOL_IP, SO_ORIGINAL_DST, unsafe.Pointer(&originalDst), &optlen)
		if err != nil {
			log.Printf("getsockopt SO_ORIGINAL_DST failed: %v", err)
			return
		}
	})

	targetAddr = net.IPv4(originalDst.SinAddr[0], originalDst.SinAddr[1], originalDst.SinAddr[2], originalDst.SinAddr[3]).String()
	targetPort = (uint16(originalDst.SinPort[0]) << 8) | uint16(originalDst.SinPort[1])
	return
}

// HTTP proxy request handler
func handleInternalConnection(conn net.Conn) {
	defer conn.Close()
	targetAddr, targetPort, err := findTargetFromConnection(conn)
	if err != nil {
		return
	}
	targetDestination := fmt.Sprintf("%s:%d", targetAddr, targetPort)
	fmt.Printf("Original destination: %s\n", targetDestination)

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", "0.0.0.0", 18001), 5*time.Second)
	if err != nil {
		log.Printf("Failed to connect to original destination: %v", err)
		return
	}
	defer targetConn.Close()
	targetConn.Write([]byte(targetDestination))
	tmp := make([]byte, 256)

	targetConn.Read(tmp)
	fmt.Printf("%s\n", tmp)
	fmt.Printf("Proxying connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

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
func handleExternalConnection(conn net.Conn) {
	defer conn.Close()
	targetAddr, targetPort, err := findTargetFromConnection(conn)
	if err != nil {
		return
	}
	fmt.Printf("External: %s:%d\n", targetAddr, targetPort)

	tmp := make([]byte, 256)
	n, err := conn.Read(tmp)
	if err != nil {
		log.Print(err)
	}

	// Check that the original destination address is reachable from the proxy
	targetConn, err := net.DialTimeout("tcp", string(tmp[:n]), 5*time.Second)
	if err != nil {
		log.Printf("Failed to connect to original destination[%s]: %v", string(tmp), err)
		return
	}
	defer targetConn.Close()
	conn.Write([]byte{'Y'})

	fmt.Printf("Proxying connection from %s to %s\n", conn.RemoteAddr(), targetConn.RemoteAddr())

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
