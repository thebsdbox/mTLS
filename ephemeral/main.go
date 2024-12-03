package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	ctx       = context.Background()
	namespace = "default"
)

func main() {
	var podName string
	flag.StringVar(&podName, "pod", "", "Pod to attach sidecar too")
	flag.Parse()
	// 0. Initialize the Kubernetes client.
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path.Join(home, ".kube/config"))
	if err != nil {
		panic(err.Error())
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Looking for pod [%s]\n", podName)
	pod, err := client.CoreV1().Pods(corev1.NamespaceDefault).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	// 2. Add an ephemeral container to the pod spec.
	podWithEphemeralContainer := withProxyContainer(pod)

	// 3. Prepare the patch.
	podJSON, err := json.Marshal(pod)
	if err != nil {
		panic(err.Error())
	}

	podWithEphemeralContainerJSON, err := json.Marshal(podWithEphemeralContainer)
	if err != nil {
		panic(err.Error())
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(podJSON, podWithEphemeralContainerJSON, pod)
	if err != nil {
		panic(err.Error())
	}

	// 4. Apply the patch.
	pod, err = client.CoreV1().
		Pods(pod.Namespace).
		Patch(
			ctx,
			pod.Name,
			types.StrategicMergePatchType,
			patch,
			metav1.PatchOptions{},
			"ephemeralcontainers",
		)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Pod has %d ephemeral containers.\n", len(pod.Spec.EphemeralContainers))
	fmt.Printf("Pod has %d volumes.\n", len(pod.Spec.Volumes))

}

func withProxyContainer(pod *corev1.Pod) *corev1.Pod {
	//privileged := true
	secret := pod.Name + "-smesh"
	//policy := corev1.ContainerRestartPolicyAlways

	// ec := &corev1.EphemeralContainer{
	// 	EphemeralContainerCommon: corev1.EphemeralContainerCommon{
	// 		Name:  "smesh-proxy",
	// 		Image: "thebsdbox/smesh-proxy:v1",
	// 		SecurityContext: &corev1.SecurityContext{
	// 			Privileged: &privileged, // TODO: Fix permissions
	// 		},
	// 		//VolumeMounts: []corev1.VolumeMount{
	// 		// {
	// 		// 	Name:      "certs",
	// 		// 	ReadOnly:  true,
	// 		// 	MountPath: "/tmp/",
	// 		// },
	// 		//},
	// 	},
	// 	//TargetContainerName: "app",
	// }
	vol := corev1.SecretVolumeSource{SecretName: secret}
	copied := pod.DeepCopy()
	//copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)
	copied.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: "certs", VolumeSource: corev1.VolumeSource{Secret: &vol}})

	return copied
}
