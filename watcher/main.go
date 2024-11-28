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
	"math/big"
	"net"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gookit/slog"
)

type certs struct {
	cacert []byte
	cakey  []byte
	key    []byte
	cert   []byte
	folder *string
}

func main() {
	var kubeconfig *string
	u, err := user.Current()
	if err != nil {
		if u != nil {
			kubeconfig = flag.String("kubeconfig", u.HomeDir+"/.kube/config", "Path to Kubernetes config")
		}
	}
	var certCollection certs

	slog.Info("Starting Certicate creation 🔏")
	ca := flag.Bool("ca", false, "Create a CA")
	certName := flag.String("cert", "", "Create a certificate from the CA")
	certCollection.folder = flag.String("certFolder", "", "Create a certificate from the CA")

	certIP := flag.String("ip", "192.168.0.1", "Create a certificate from the CA")
	certSecret := flag.Bool("load", false, "Create a secret in Kubernetes with the certificate")
	loadCA := flag.Bool("loadca", false, "Create a secret in Kubernetes with the certificate")
	watch := flag.Bool("watch", false, "Watch Kubernetes for pods being created and create certs")
	flag.Parse()

	if *ca {
		err := certCollection.generateCA()
		if err != nil {
			panic(err)
		}
		err = certCollection.writeCACert()
		if err != nil {
			panic(err)
		}
		err = certCollection.writeCAKey()
		if err != nil {
			panic(err)
		}
	}
	if *loadCA {
		err := certCollection.readCACert()
		if err != nil {
			slog.PanicErr(err)
		}
		err = certCollection.readCAKey()
		if err != nil {
			slog.PanicErr(err)
		}

		c, err := client(*kubeconfig)
		if err != nil {
			slog.PanicErr(err)
		}
		err = certCollection.loadCA(c)
		if err != nil {
			slog.PanicErr(err)
		}
	}
	if *certName != "" {
		certCollection.createCertificate(*certName, *certIP)
		err := certCollection.writeCert(*certName)
		if err != nil {
			panic(err)
		}
		err = certCollection.writeKey(*certName)
		if err != nil {
			panic(err)
		}
		if *certSecret {
			c, err := client(*kubeconfig)
			if err != nil {
				slog.PanicErr(err)
			}
			err = certCollection.loadSecret(*certName, c)
			if err != nil {
				slog.Error("secret", "msg", err)
			}
		}
	}
	if *watch {
		err := certCollection.getEnvCerts()
		if err != nil {
			slog.Warnf("Error reading certificates from env vars [%v]", err)

			err := certCollection.readCACert()
			if err != nil {
				slog.PanicErr(err)
			}
			err = certCollection.readCAKey()
			if err != nil {
				slog.PanicErr(err)
			}
		}
		var c *kubernetes.Clientset
		if kubeconfig == nil {
			c, err = client("")

		} else {
			c, err = client(*kubeconfig)
		}
		if err != nil {
			slog.PanicErr(err)
		}
		certCollection.watcher(c)
	}

}

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

func (c *certs) readCACert() (err error) {
	c.cacert, err = os.ReadFile(*c.folder + "ca.crt")
	if err != nil {
		return err
	}
	return nil
}

func (c *certs) readCAKey() (err error) {
	c.cakey, err = os.ReadFile(*c.folder + "ca.key")
	if err != nil {
		return err
	}
	return nil
}

func (c *certs) writeCACert() (err error) {
	// Public key
	certOut, err := os.Create(*c.folder + "ca.crt")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cacert)
	certOut.Close()
	slog.Info("written ca.crt")
	return nil
}

func (c *certs) writeCAKey() (err error) {
	// Public key
	certOut, err := os.Create(*c.folder + "ca.key")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cakey)
	certOut.Close()
	slog.Info("written ca.key")
	return nil
}

func (c *certs) writeCert(name string) (err error) {
	// Public key
	certOut, err := os.Create(name + ".crt")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.cert)
	certOut.Close()
	slog.Info("written ca.crt")
	return nil
}

func (c *certs) writeKey(name string) (err error) {
	// Public key
	certOut, err := os.Create(name + ".key")
	if err != nil {
		slog.Error("create ca failed", err)
		return err
	}
	certOut.Write(c.key)
	certOut.Close()
	slog.Info("written ca.key")
	return nil
}

func (c *certs) generateCA() error {
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
		slog.Error("create ca failed")
		return err
	}

	// Public key
	// certOut, err := os.Create("ca.crt")
	// if err != nil {
	// 	slog.Error("create ca failed", err)
	// 	return err
	// }
	c.cacert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca_b})
	//pem.Encode(certOut, )
	//certOut.Close()
	//slog.Info("written ca.crt")

	// Private key
	// keyOut, err := os.OpenFile("ca.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	// if err != nil {
	// 	slog.Error("create ca failed", err)
	// 	return err
	// }
	c.cakey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	//pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// keyOut.Close()
	// slog.Info("written ca.key")
	return nil
}

func (c *certs) createCertificate(name, ip string) {
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
	ipAddress := net.ParseIP(ip)
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
		DNSNames:     []string{name},
		IPAddresses:  []net.IP{ipAddress},
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey

	// Sign the certificate
	cert_b, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, catls.PrivateKey)
	if err != nil {
		panic(err)
	}
	// Public key
	// certOut, err := os.Create(certificate)
	// if err != nil {
	// 	panic(err)
	// }
	c.cert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert_b})
	// pem.Encode(certOut)
	// certOut.Close()
	// slog.Info(fmt.Sprintf("Written %s", certificate))
	c.key = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// Private key
	// keyOut, err := os.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	// if err != nil {
	// 	panic(err)
	// }
	// pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	// keyOut.Close()
	// slog.Info(fmt.Sprintf("Written %s", key))

}

func client(kubeconfigPath string) (*kubernetes.Clientset, error) {
	var kubeconfig *rest.Config

	if kubeconfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load kubeconfig from %s: %v", kubeconfigPath, err)
		}
		kubeconfig = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("unable to load in-cluster config: %v", err)
		}
		kubeconfig = config
	}

	// build the client set
	clientSet, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("creating the kubernetes client set - %s", err)
	}
	return clientSet, nil
}

func (c *certs) loadSecret(name string, clientSet *kubernetes.Clientset) error {

	// certificate := fmt.Sprint(name + ".crt")
	// key := fmt.Sprint(name + ".key")
	// certData, err := os.ReadFile(certificate)
	// if err != nil {
	// 	return fmt.Errorf("unable to read certificate %v", err)
	// }
	// keyData, err := os.ReadFile(key)
	// if err != nil {
	// 	return fmt.Errorf("unable to read key %v", err)
	// }
	// caData, err := os.ReadFile("ca.crt")
	// if err != nil {
	// 	return fmt.Errorf("unable to read ca %v", err)
	// }

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

	s, err := clientSet.CoreV1().Secrets(v1.NamespaceDefault).Create(context.TODO(), &secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("unable to create secrets %v", err)
	}
	slog.Info(fmt.Sprintf("Created Secret 🔐 [%s]", s.Name))

	return nil

	return nil
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
		i.c.createCertificate(newPod.Name, newPod.Status.PodIP)

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
	// p := obj.(*corev1.Pod)

	// // dp := obj.(*v1.Deployment)
	// // if dp.ObjectMeta.Annotations["needFluentd"] == "yes" {

	// p2, err := i.clientset.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
	// if err != nil {
	// 	klog.Infoln(err)
	// }

	// klog.Infof("ADD: the old version %s %s %s", p2.Name, p2.ObjectMeta.ResourceVersion, p2.Status.PodIP)
	// fluentContainer := corev1.EphemeralContainer{
	// 	EphemeralContainerCommon: corev1.EphemeralContainerCommon{
	// 		Name:  "fluentd-sidecar",
	// 		Image: "fluent/fluentd:v1.15-debian-1",
	// 		Env: []corev1.EnvVar{
	// 			{
	// 				Name:  "FLUENTD_CONF",
	// 				Value: "fluentd.conf",
	// 			},
	// 		},
	// 	},
	// }

	// p2.Spec.EphemeralContainers = append(p2.Spec.EphemeralContainers, fluentContainer)
	// //dp2.Spec.Template.Spec.Volumes = append(dp2.Spec.Template.Spec.Volumes, fluentVolumne)
	// p2, err = i.clientset.CoreV1().Pods(p2.Namespace).Update(context.Background(), p2, metav1.UpdateOptions{})
	// if err != nil {
	// 	klog.Infoln(err)
	// }

	// p = p2.DeepCopy()
	// klog.Infof("ADD: the new version %s %s", p.Name, p.ObjectMeta.ResourceVersion)

	// // resourceVersion should not be set on objects to be created
	// // 可能的处理方法 需要先更新这个deploy

	// // _, err := i.clientset.AppsV1().Deployments(dp.Namespace).Create(context.Background(), dp, metav1.CreateOptions{})
	// // if err != nil {

	// // 	klog.Infoln(err)
	// // }
}
