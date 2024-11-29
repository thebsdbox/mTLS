package main

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type Config mirrors ./ebpf/mirrors.c

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/docker/docker/pkg/parsers/kernel"
	"github.com/gookit/slog"
)

const (
	SO_ORIGINAL_DST = 80 // Socket option to get the original destination address
)

type Config struct {
	ProxyPort      int
	ClusterPort    int
	ClusterTLSPort int
	Address        string
	ClusterAddress string // For Debug purposes
	CgroupOverride string // For Debug purposes

	PodCIDR      string
	Certificates *certs

	Socks *ebpf.Map
}

// SockAddrIn is a struct to hold the sockaddr_in structure for IPv4 "retrieved" by the SO_ORIGINAL_DST.
type SockAddrIn struct {
	SinFamily uint16
	SinPort   [2]byte
	SinAddr   [4]byte
	// Pad to match the size of sockaddr_in
	Pad [8]byte
}

func main() {
	var c Config
	flag.StringVar(&c.Address, "address", "127.0.0.1", "Address to bind to, can also be a hostname")
	flag.StringVar(&c.ClusterAddress, "overrideAddress", "", "Address to force all traffic to")
	flag.StringVar(&c.CgroupOverride, "cgroupPath", "/sys/fs/cgroup", "Path for cgroup")
	flag.IntVar(&c.ProxyPort, "proxyPort", 18000, "Port for internal proxy")
	flag.IntVar(&c.ClusterPort, "clusterPort", 18001, "External port for cluster connectivity")
	flag.IntVar(&c.ClusterTLSPort, "clusterTLSPort", 18443, "External port for cluster connectivity (TLS)")
	flag.StringVar(&c.PodCIDR, "podCIDR", "10.244.0.0/16", "The CIDR range used for POD IP addresses")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("Starting the SMESH 🐝")

	// Lookup for environment variable
	envAddress, exists := os.LookupEnv("KUBE_NODE_NAME")
	if exists {
		c.ClusterAddress = envAddress
	}
	_, gateway := os.LookupEnv("KUBE_GATEWAY")

	i, err := net.ResolveIPAddr("", c.Address)
	if err != nil {
		panic(err)
	}
	c.Address = i.String()

	// Overwrite the podcidr
	podCIDR, exists := os.LookupEnv("POD_CIDR")
	if exists {
		c.PodCIDR = podCIDR
	}

	if !gateway {

		v, err := kernel.GetKernelVersion()
		if err != nil {
			slog.Errorf("unable to parse kernel version %v", err)
		}

		slog.Infof("detected Kernel %d.%d.x", v.Kernel, v.Major)

		// Remove resource limits for kernels <5.11.
		if v.Kernel >= 5 && v.Major >= 11 {
			if err := rlimit.RemoveMemlock(); err != nil {
				slog.Print("Removing memlock:", err)
			}
		}

		// Load the compiled eBPF ELF and load it into the kernel
		// NOTE: we could also pin the eBPF program
		var objs mirrorsObjects
		if err := loadMirrorsObjects(&objs, nil); err != nil {
			slog.Print("Error loading eBPF objects:", err)
		}
		defer objs.Close()
		// Attach eBPF programs to the root cgroup
		connect4Link, err := link.AttachCgroup(link.CgroupOptions{
			Path:    c.CgroupOverride,
			Attach:  ebpf.AttachCGroupInet4Connect,
			Program: objs.CgConnect4,
		})
		if err != nil {
			slog.Print("Attaching CgConnect4 program to Cgroup:", err)
		}
		defer connect4Link.Close()

		sockopsLink, err := link.AttachCgroup(link.CgroupOptions{
			Path:    c.CgroupOverride,
			Attach:  ebpf.AttachCGroupSockOps,
			Program: objs.CgSockOps,
		})
		if err != nil {
			slog.Print("Attaching CgSockOps program to Cgroup:", err)
		}
		defer sockopsLink.Close()

		sockoptLink, err := link.AttachCgroup(link.CgroupOptions{
			Path:    c.CgroupOverride,
			Attach:  ebpf.AttachCGroupGetsockopt,
			Program: objs.CgSockOpt,
		})
		if err != nil {
			slog.Print("Attaching CgSockOpt program to Cgroup:", err)
		}
		defer sockoptLink.Close()

		cidr := strings.Split(c.PodCIDR, "/")
		if len(cidr) < 1 {
			panic(fmt.Errorf("error parsing cidr %s", c.PodCIDR))
		}

		// Update the proxyMaps map with the proxy server configuration, because we need to know the proxy server PID in order
		// to filter out eBPF events generated by the proxy server itself so it would not proxy its own packets in a loop.
		var key uint32 = 0
		i, err := strconv.Atoi(cidr[1])
		if err != nil {
			// ... handle error
			panic(err)
		}
		config := mirrorsConfig{
			ProxyPort: uint16(c.ProxyPort),
			ProxyPid:  uint64(os.Getpid()),
			ProxyAddr: uint32(ToInt(c.Address)),
			Network:   uint32(ToInt(cidr[0])),
			Mask:      uint16(i),
		}

		err = objs.mirrorsMaps.MapConfig.Update(&key, &config, ebpf.UpdateAny)
		if err != nil {
			slog.Fatalf("Failed to update proxyMaps map: %v", err)
		}

		// Start the proxy server on the localhost
		// We only demonstrate IPv4 in this example, but the same approach can be used for IPv6
		c.Socks = objs.MapSocks
		internalListener := c.startInternalListener()
		defer internalListener.Close()
		go c.start(internalListener, true)

	}

	externalListener := c.startExternalListener()
	defer externalListener.Close()

	// Attempt to get certificates from API
	// c.Certificates, err = getKubeCerts(os.Getenv("KUBECONFIG"))
	// if err != nil {
	// 	slog.Error(err)
	// Attempt to get from environment secrets
	c.Certificates, err = getEnvCerts()
	if err != nil {
		slog.Error(err)
	}
	//}
	// If we have secrets enable a TLS listener
	if c.Certificates != nil {
		externalTLSListener := c.startExternalTLSListener()
		defer externalTLSListener.Close()
		go c.startTLS(externalTLSListener)
	}

	go c.start(externalListener, false)
	_, exists = os.LookupEnv("DEBUG")
	if exists {
		go cat()
	}
	<-ctx.Done() // We wait here

}

func readLines(r io.Reader) {
	rd := bufio.NewReader(r)
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(fmt.Sprintf("Error reading lines %v", err))
		}

		fmt.Printf("%s", line)

	}
}

func cat() {
	file, err := os.Open("/sys/kernel/debug/tracing/trace_pipe")
	if err != nil {
		fmt.Printf("Error trace pipe %v\n", err)
		return
	}
	defer file.Close()
	readLines(file)
}
