package main

import (
	"context"
	"fmt"
	"smesh/pkg/manager"

	"github.com/gookit/slog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeclient *kubernetes.Clientset

func main() {

	slog.Info("starting the SMESH 🐝")
	c, err := manager.Setup()
	if err != nil {
		slog.Fatal(err)
	}
	err = manager.LoadEPF(c)
	if err != nil {
		slog.Fatal(err)
	}
	c.Proxy = true
	c.ProxyFunc = getHost
	err = manager.Start(c)
	if err != nil {
		slog.Fatal(err)
	}
}

func getHost(ip string) string {
	opts := metav1.ListOptions{}
	opts.FieldSelector = "status.podIP=" + ip
	l, _ := kubeclient.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), opts)
	//fmt.Printf("Pods %d %s \n", len(l.Items))
	//fmt.Print(l)
	//client.CoreV1().Endpoints(v1.NamespaceAll).Get()
	if len(l.Items) == 0 {
		return ""
	}
	if len(l.Items) > 1 {
		slog.Warnf("Found [%d] pods with the address [%s]", len(l.Items), ip)
	}
	// hmmm should really only ever be one
	return l.Items[0].Status.HostIP
}

func client(kubeconfigPath string) error {
	var kubeconfig *rest.Config
	var err error
	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
		}
		kubeconfig = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("unable to load in-cluster config: %v", err)
		}
		kubeconfig = config
	}

	// build the client set
	kubeclient, err = kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("creating the kubernetes client set - %s", err)
	}
	return nil
}
