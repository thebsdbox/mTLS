package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gookit/slog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

// This contains the watcher for new pods, sadly we can't do this in the webhook
// this is because we need to wait for the pods to have been allocated IP an
// IP address

type certs struct {
	cacert    []byte
	cakey     []byte
	key       []byte
	cert      []byte
	org       string
	namespace string
}

// Actual watcher code

type informerHandler struct {
	clientset *kubernetes.Clientset
	c         *certs
}

func (c *certs) watcher(clientSet *kubernetes.Clientset) error {

	factory := informers.NewSharedInformerFactory(clientSet, 0)

	informer := factory.Core().V1().Pods().Informer()

	_, err := informer.AddEventHandler(&informerHandler{clientset: clientSet, c: c})
	if err != nil {
		return err
	}
	stop := make(chan struct{}, 2)

	go informer.Run(stop)
	forever := make(chan os.Signal, 1)
	signal.Notify(forever, syscall.SIGINT, syscall.SIGTERM)
	<-forever
	stop <- struct{}{}
	close(forever)
	close(stop)
	return nil
}

func (i *informerHandler) OnUpdate(oldObj, newObj interface{}) {
	newPod := newObj.(*v1.Pod)
	oldPod := oldObj.(*v1.Pod)

	// Inspect the changes
	if oldPod.Status.PodIP != newPod.Status.PodIP && newPod.Status.PodIP != "" {
		i.c.createCertificate(newPod.Name, []string{newPod.Name}, &newPod.Status.PodIP)

		err := i.c.loadSecret(newPod.Name, i.clientset)
		if err != nil {
			slog.Error(err)
		}
		//loadSecret(newPod.Name, "", i.clientset)
	}
}

func (i *informerHandler) OnDelete(obj interface{}) {
	p := obj.(*v1.Pod)
	name := fmt.Sprintf("%s-smesh", p.Name)
	err := i.clientset.CoreV1().Secrets(p.Namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	if err != nil {
		slog.Errorf("Error deleting secret %v", err)
	} else {
		slog.Infof("Deleted secret 🔏 [%s]", name)
	}

}

func (i *informerHandler) OnAdd(obj interface{}, b bool) {
}

// -- cert management code --

func (c *certs) getEnvCerts() (err error) {
	envcert, exists := os.LookupEnv("SMESH-CA-CERT")
	if !exists {
		return fmt.Errorf("unable to find secrets from environment")
	}
	envkey, exists := os.LookupEnv("SMESH-CA-KEY")
	if !exists {
		return fmt.Errorf("unable to find secrets from environment")
	}
	c.cacert = []byte(envcert)
	c.cakey = []byte(envkey)
	return nil
}

func (c *certs) generateCA() error {
	// ca := &x509.Certificate{
	// 	SerialNumber: big.NewInt(1653),
	// 	Subject: pkix.Name{
	// 		Organization:  []string{"ORGANIZATION_NAME"},
	// 		Country:       []string{"COUNTRY_CODE"},
	// 		Province:      []string{"PROVINCE"},
	// 		Locality:      []string{"CITY"},
	// 		StreetAddress: []string{"ADDRESS"},
	// 		PostalCode:    []string{"POSTAL_CODE"},
	// 		CommonName:    "42CA",
	// 	},
	// 	NotBefore:             time.Now(),
	// 	NotAfter:              time.Now().AddDate(10, 0, 0),
	// 	IsCA:                  true,
	// 	ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
	// 	KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	// 	BasicConstraintsValid: true,
	// }

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(2022),
		Subject:               pkix.Name{Organization: []string{c.org}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // expired in 1 year
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	ca_b, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		slog.Error("create ca failed")
		return err
	}

	// Public key
	c.cacert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca_b})

	// Private key
	c.cakey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return nil
}

func (c *certs) loadCA(clientSet *kubernetes.Clientset) error {
	secretMap := make(map[string][]byte)

	secretMap["ca-cert"] = c.cacert
	secretMap["ca-key"] = c.cakey
	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "watcher",
		},
		Data: secretMap,
		Type: v1.SecretTypeOpaque,
	}

	s, err := clientSet.CoreV1().Secrets(c.namespace).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create secrets %v", err)
	}
	slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))

	return nil
}

func (c *certs) createCertificate(commonname string, dnsNames []string, ip *string) {
	// Load CA
	tls.X509KeyPair(c.cacert, c.cakey)
	catls, err := tls.X509KeyPair(c.cacert, c.cakey)
	if err != nil {
		panic(err)
	}
	ca, err := x509.ParseCertificate(catls.Certificate[0])
	if err != nil {
		panic(err)
	}
	// Prepare certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			Organization: []string{c.org},
			CommonName:   commonname,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames:     dnsNames,
	}

	// if name != nil {
	// 	cert.DNSNames = append(cert.DNSNames, *name)
	// }
	if ip != nil {
		ipAddress := net.ParseIP(*ip)
		cert.IPAddresses = append(cert.IPAddresses, ipAddress)
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey

	// Sign the certificate
	cert_b, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, catls.PrivateKey)
	if err != nil {
		panic(err)
	}
	// Public key
	c.cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert_b})

	// Private key
	c.key = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

}

func (c *certs) loadSecret(name string, clientSet *kubernetes.Clientset) error {
	secretMap := make(map[string][]byte)

	secretMap["ca"] = c.cacert
	secretMap["cert"] = c.cert
	secretMap["key"] = c.key

	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name + "-smesh",
		},
		Data: secretMap,
		Type: v1.SecretTypeOpaque,
	}

	s, err := clientSet.CoreV1().Secrets(v1.NamespaceDefault).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create secrets %v", err)
	}
	slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))

	return nil
}
