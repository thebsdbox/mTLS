package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"

	"github.com/gookit/slog"
)

type certs struct {
	ca   []byte
	key  []byte
	cert []byte
}

// helper function for getsockopt
func getsockopt(s int, level int, optname int, optval unsafe.Pointer, optlen *uint32) (err error) {
	_, _, e := syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(s), uintptr(level), uintptr(optname), uintptr(optval), uintptr(unsafe.Pointer(optlen)), 0)
	if e != 0 {
		return e
	}
	return
}

func ToInt(address string) int {
	ip := net.ParseIP(address)
	i := int(ip[12]) * 16777216
	i += int(ip[13]) * 65536
	i += int(ip[14]) * 256
	i += int(ip[15])
	return i
}

func (c *Config) findTargetFromConnection(conn net.Conn) (targetAddr string, targetPort uint16, err error) {
	// Using RawConn is necessary to perform low-level operations on the underlying socket file descriptor in Go.
	// This allows us to use getsockopt to retrieve the original destination address set by the SO_ORIGINAL_DST option,
	// which isn't directly accessible through Go's higher-level networking API.
	rawConn, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		slog.Printf("Failed to get raw connection: %v", err)
		return
	}

	var originalDst SockAddrIn
	// var cookie uint64
	// If Control is not nil, it is called after creating the network connection but before binding it to the operating system.
	rawConn.Control(func(fd uintptr) {
		optlen := uint32(unsafe.Sizeof(originalDst))
		// Retrieve the original destination address by making a syscall with the SO_ORIGINAL_DST option.
		err = getsockopt(int(fd), syscall.SOL_IP, SO_ORIGINAL_DST, unsafe.Pointer(&originalDst), &optlen)
		if err != nil {
			slog.Printf("getsockopt SO_ORIGINAL_DST failed: %v", err)
			return
		}
		// cookie, err = unix.GetsockoptUint64(int(fd), unix.SOL_SOCKET, unix.SO_COOKIE)
		// if err != nil {
		// 	slog.Printf("getsockopt SOL_SOCKET failed: %v", err)
		// }
	})
	// slog.Info("🍪 %d", cookie)
	// i := c.Socks.Iterate()
	// var key uint32
	// var value mirrorsSocket
	// for i.Next(&key, &value) {
	// 	// Order of keys is non-deterministic due to randomized map seed
	// 	slog.Infof("%d %v", key, value)
	// }

	// // if err != nil || err2 != nil {
	// // 	return
	// // }
	// // slog.Infof("Cookies %d", cookie)
	// var m mirrorsSocket
	// err = c.Socks.Lookup(uint32(cookie), &m)
	// if err != nil {
	// 	slog.Error(err)
	// } else {
	// 	fmt.Printf("%v", m)
	// }
	targetAddr = net.IPv4(originalDst.SinAddr[0], originalDst.SinAddr[1], originalDst.SinAddr[2], originalDst.SinAddr[3]).String()
	targetPort = (uint16(originalDst.SinPort[0]) << 8) | uint16(originalDst.SinPort[1])
	return
}

func getEnvCerts() (*certs, error) {
	envca, exists := os.LookupEnv("SMESH-CA")
	if !exists {
		return nil, fmt.Errorf("unable to find secrets from environment")
	}
	envcert, exists := os.LookupEnv("SMESH-CERT")
	if !exists {
		return nil, fmt.Errorf("unable to find secrets from environment")
	}
	envkey, exists := os.LookupEnv("SMESH-KEY")
	if !exists {
		return nil, fmt.Errorf("unable to find secrets from environment")
	}
	return &certs{
		ca:   []byte(envca),
		cert: []byte(envcert),
		key:  []byte(envkey),
	}, nil

}
