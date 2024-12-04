package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gookit/slog"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	webhookConfigName = "sidecar-injector-webhook"
	webhookInjectPath = "/inject"
)

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

func createOrUpdateMutatingWebhookConfiguration(caPEM []byte, webhookService, webhookNamespace string, clientset *kubernetes.Clientset) error {
	slog.Println("Initializing the kube client...")

	mutatingWebhookConfigV1Client := clientset.AdmissionregistrationV1()
	slog.Printf("Creating or updating the mutatingwebhookconfiguration: %s", webhookConfigName)
	fail := admissionregistrationv1.Fail
	sideEffect := admissionregistrationv1.SideEffectClassNone

	mutatingWebhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookConfigName,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			Name:                    "sidecar-injector.thebsdbox.co.uk",
			AdmissionReviewVersions: []string{"v1", "v1beta1"},
			SideEffects:             &sideEffect,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				CABundle: caPEM, // self-generated CA for the webhook
				Service: &admissionregistrationv1.ServiceReference{
					Name:      webhookService,
					Namespace: webhookNamespace,
					Path:      &webhookInjectPath,
				},
			},
			Rules: []admissionregistrationv1.RuleWithOperations{
				{
					Operations: []admissionregistrationv1.OperationType{
						admissionregistrationv1.Create,
						admissionregistrationv1.Update,
					},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"pods"},
					},
				},
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"sidecar-injection": "enabled",
				},
			},
			FailurePolicy: &fail,
		}},
	}

	mutatewebhook, _ := json.Marshal(mutatingWebhookConfig)

	fmt.Println("---------------------------------")
	fmt.Println("New MutatingWebhookConfig Object")
	fmt.Println("---------------------------------")
	fmt.Println(string(mutatewebhook))

	foundWebhookConfig, err := mutatingWebhookConfigV1Client.MutatingWebhookConfigurations().Get(context.TODO(), webhookConfigName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		if _, err := mutatingWebhookConfigV1Client.MutatingWebhookConfigurations().Create(context.TODO(), mutatingWebhookConfig, metav1.CreateOptions{}); err != nil {
			slog.Warnf("Failed to create the mutatingwebhookconfiguration: %s", webhookConfigName)
			return err
		}
		slog.Printf("Created mutatingwebhookconfiguration: %s", webhookConfigName)
	} else if err != nil {
		slog.Warnf("Failed to check the mutatingwebhookconfiguration: %s", webhookConfigName)
		return err
	} else {
		// there is an existing mutatingWebhookConfiguration
		if len(foundWebhookConfig.Webhooks) != len(mutatingWebhookConfig.Webhooks) ||
			!(foundWebhookConfig.Webhooks[0].Name == mutatingWebhookConfig.Webhooks[0].Name &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].AdmissionReviewVersions, mutatingWebhookConfig.Webhooks[0].AdmissionReviewVersions) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].SideEffects, mutatingWebhookConfig.Webhooks[0].SideEffects) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].FailurePolicy, mutatingWebhookConfig.Webhooks[0].FailurePolicy) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].Rules, mutatingWebhookConfig.Webhooks[0].Rules) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].NamespaceSelector, mutatingWebhookConfig.Webhooks[0].NamespaceSelector) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].ClientConfig.CABundle, mutatingWebhookConfig.Webhooks[0].ClientConfig.CABundle) &&
				reflect.DeepEqual(foundWebhookConfig.Webhooks[0].ClientConfig.Service, mutatingWebhookConfig.Webhooks[0].ClientConfig.Service)) {
			mutatingWebhookConfig.ObjectMeta.ResourceVersion = foundWebhookConfig.ObjectMeta.ResourceVersion
			if _, err := mutatingWebhookConfigV1Client.MutatingWebhookConfigurations().Update(context.TODO(), mutatingWebhookConfig, metav1.UpdateOptions{}); err != nil {
				slog.Warnf("Failed to update the mutatingwebhookconfiguration: %s, %v", webhookConfigName)
				return err
			}
			slog.Printf("Updated the mutatingwebhookconfiguration: %s", webhookConfigName)
		}
		slog.Warnf("The mutatingwebhookconfiguration: %s already exists and has no change", webhookConfigName)
	}

	// fmt.Println(mutatingWebhookConfig)

	return nil
}

func tidyWebhook(webhookConfigName string, clientset *kubernetes.Clientset) error {
	mutatingWebhookConfigV1Client := clientset.AdmissionregistrationV1()

	return mutatingWebhookConfigV1Client.MutatingWebhookConfigurations().Delete(context.TODO(), webhookConfigName, *metav1.NewDeleteOptions(0))

}
