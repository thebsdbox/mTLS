package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/user"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var log *slog.Logger

func main() {
	u, _ := user.Current()

	log = slog.New(slog.NewTextHandler(os.Stdout, nil))
	log.Info("Starting Certicate creation 🔏")
	ca := flag.Bool("ca", false, "Create a CA")
	certName := flag.String("cert", "", "Create a certificate from the CA")
	certSecret := flag.String("secret", "", "Create a secret in Kubernetes with the certificate")
	kubeconfig := flag.String("kubeconfig", u.HomeDir+"/.kube/config", "Path to Kubernetes config")

	flag.Parse()
	if *ca {
		createCA()
	}
	if *certName != "" {
		createCertificate(*certName)
	}
	if *certSecret != "" {
		err := loadSecret(*certSecret, *kubeconfig)
		if err != nil {
			log.Error("secret", "msg", err)
		}
	}
}

func createCA() error {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1653),
		Subject: pkix.Name{
			Organization:  []string{"ORGANIZATION_NAME"},
			Country:       []string{"COUNTRY_CODE"},
			Province:      []string{"PROVINCE"},
			Locality:      []string{"CITY"},
			StreetAddress: []string{"ADDRESS"},
			PostalCode:    []string{"POSTAL_CODE"},
			CommonName:    "42CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	ca_b, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, priv)
	if err != nil {
		log.Error("create ca failed")
		return err
	}

	// Public key
	certOut, err := os.Create("ca.crt")
	if err != nil {
		log.Error("create ca failed", err)
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: ca_b})
	certOut.Close()
	log.Info("written ca.crt")

	// Private key
	keyOut, err := os.OpenFile("ca.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Error("create ca failed", err)
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	log.Info("written ca.key")
	return nil
}

func createCertificate(name string) {
	certificate := fmt.Sprint(name + ".crt")
	key := fmt.Sprint(name + ".key")
	// Load CA
	catls, err := tls.LoadX509KeyPair("ca.crt", "ca.key")
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
			Organization:  []string{"ORGANIZATION_NAME"},
			Country:       []string{"COUNTRY_CODE"},
			Province:      []string{"PROVINCE"},
			Locality:      []string{"CITY"},
			StreetAddress: []string{"ADDRESS"},
			PostalCode:    []string{"POSTAL_CODE"},
			CommonName:    "TEST",
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey

	// Sign the certificate
	cert_b, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, catls.PrivateKey)
	if err != nil {
		panic(err)
	}
	// Public key
	certOut, err := os.Create(certificate)
	if err != nil {
		panic(err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: cert_b})
	certOut.Close()
	log.Info(fmt.Sprintf("Written %s", certificate))

	// Private key
	keyOut, err := os.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		panic(err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	log.Info(fmt.Sprintf("Written %s", key))

}

func loadSecret(name, kubeconfigPath string) error {
	var kubeconfig *rest.Config

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
	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("creating the kubernetes client set - %s", err)
	}

	certificate := fmt.Sprint(name + ".crt")
	key := fmt.Sprint(name + ".key")
	certData, err := os.ReadFile(certificate)
	if err != nil {
		return fmt.Errorf("unable to read certificate %v", err)
	}
	keyData, err := os.ReadFile(key)
	if err != nil {
		return fmt.Errorf("unable to read key %v", err)
	}
	caData, err := os.ReadFile("ca.crt")
	if err != nil {
		return fmt.Errorf("unable to read ca %v", err)
	}

	secretMap := make(map[string][]byte)

	secretMap["ca"] = caData
	secretMap["cert"] = certData
	secretMap["key"] = keyData

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
	log.Info(fmt.Sprintf("Created Secret %s", s.Name))

	return nil
}
