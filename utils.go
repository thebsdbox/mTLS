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

func findTargetFromConnection(conn net.Conn) (targetAddr string, targetPort uint16, err error) {
	// Using RawConn is necessary to perform low-level operations on the underlying socket file descriptor in Go.
	// This allows us to use getsockopt to retrieve the original destination address set by the SO_ORIGINAL_DST option,
	// which isn't directly accessible through Go's higher-level networking API.
	rawConn, err := conn.(*net.TCPConn).SyscallConn()
	if err != nil {
		slog.Printf("Failed to get raw connection: %v", err)
		return
	}

	var originalDst SockAddrIn
	// If Control is not nil, it is called after creating the network connection but before binding it to the operating system.
	rawConn.Control(func(fd uintptr) {
		optlen := uint32(unsafe.Sizeof(originalDst))
		// Retrieve the original destination address by making a syscall with the SO_ORIGINAL_DST option.
		err = getsockopt(int(fd), syscall.SOL_IP, SO_ORIGINAL_DST, unsafe.Pointer(&originalDst), &optlen)
		if err != nil {
			slog.Printf("getsockopt SO_ORIGINAL_DST failed: %v", err)
			return
		}
	})

	targetAddr = net.IPv4(originalDst.SinAddr[0], originalDst.SinAddr[1], originalDst.SinAddr[2], originalDst.SinAddr[3]).String()
	targetPort = (uint16(originalDst.SinPort[0]) << 8) | uint16(originalDst.SinPort[1])
	return
}

// func getKubeCerts(kubeconfigPath string) (*certs, error) {
// 	// ClientSet from Inside

// 	var kubeconfig *rest.Config

// 	if kubeconfigPath != "" {
// 		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
// 		if err != nil {
// 			return nil, fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
// 		}
// 		kubeconfig = config
// 	} else {
// 		config, err := rest.InClusterConfig()
// 		if err != nil {
// 			return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
// 		}
// 		kubeconfig = config
// 	}

// 	// build the client set
// 	clientSet, err := kubernetes.NewForConfig(kubeconfig)
// 	if err != nil {
// 		return nil, fmt.Errorf("creating the kubernetes client set - %s", err)
// 	}
// 	hostname, err := os.Hostname()
// 	if err != nil {
// 		return nil, fmt.Errorf("unable to determine hostname %v", err)
// 	}
// 	secret, err := clientSet.CoreV1().Secrets(v1.NamespaceDefault).Get(context.TODO(), fmt.Sprintf("%s-smesh", hostname), metav1.GetOptions{})
// 	if err != nil {
// 		return nil, fmt.Errorf("unable get secrets %v", err)
// 	}
// 	return &certs{ca: secret.Data["ca"], cert: secret.Data["cert"], key: secret.Data["key"]}, nil
// }

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
