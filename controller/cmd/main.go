package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/gookit/slog"
)

var (
	port               int
	webhookServiceName string
)
var c certs

func init() {
	c.namespace = os.Getenv("POD_NAMESPACE")
}

func main() {
	// init command flags
	var kubeconfig string
	u, err := user.Current()
	if err == nil {
		if u != nil {
			flag.StringVar(&kubeconfig, "kubeconfig", u.HomeDir+"/.kube/config", "Path to Kubernetes config")
		}
	}

	flag.IntVar(&port, "port", 8443, "Webhook server port.")
	flag.StringVar(&webhookServiceName, "service-name", "sidecar-injector", "Webhook service name.")
	flag.Parse()

	client, err := client(kubeconfig)
	if err != nil {
		slog.Fatalf("unable to get Kubernetes client [%v]", err)
	}

	dnsNames := []string{
		webhookServiceName,
		webhookServiceName + "." + c.namespace,
		webhookServiceName + "." + c.namespace + ".svc",
	}
	commonName := webhookServiceName + "." + c.namespace + ".svc"



	c.org = "thebsdbox.co.uk"
	err = c.getEnvCerts()
	if err != nil {
		slog.Errorf("Error loading existing certificates, generating new ones [%v]", err)
		err = c.generateCA()
		if err != nil {
			slog.Fatalf("generating CA [%v]", err)
		}
		err = c.loadCA(client)
		if err != nil {
			slog.Fatalf("creating secrets for CA [%v]", err)
		}
	}

	// Create our client certificates to authenticate with the API Server
	c.createCertificate(commonName, dnsNames, nil)

	if err != nil {
		slog.Fatalf("Failed to generate ca and certificate key pair: %v", err)
	}

	pair, err := tls.X509KeyPair(c.cert, c.key)
	if err != nil {
		slog.Fatalf("Failed to load certificate key pair: %v", err)
	}

	// create or update the mutatingwebhookconfiguration
	err = createOrUpdateMutatingWebhookConfiguration(c.cacert, webhookServiceName, c.namespace, client)
	if err != nil {
		slog.Fatalf("Failed to create or update the mutating webhook configuration: %v", err)
	}

	whsvr := &WebhookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v", port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
	}

	go c.watcher(client)

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc(webhookInjectPath, whsvr.serve)
	whsvr.server.Handler = mux

	// start webhook server in new rountine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			slog.Fatalf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	slog.Printf("Got OS shutdown signal, shutting down webhook server gracefully...")
	whsvr.server.Shutdown(context.Background())
	err = tidyWebhook(webhookConfigName, client)
	if err != nil {
		slog.Errorf("unable to remove webhook configuration [%v]", err)
	}
}
