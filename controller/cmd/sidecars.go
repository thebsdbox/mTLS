package main

import (
	corev1 "k8s.io/api/core/v1"
)

func smeshproxy(podname string) *corev1.Container {
	privileged := true
	secret := podname + "-smesh"
	policy := corev1.ContainerRestartPolicyAlways
	c := &corev1.Container{
		Name:  "smesh-proxy",
		Image: "thebsdbox/smesh-proxy:v1",
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged, // TODO: Fix permissions
		},
		RestartPolicy: &policy,
		Env: []corev1.EnvVar{
			{
				Name: "SMESH-CA",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret,
						},
						Key: "ca",
					},
				},
			},
			{
				Name: "SMESH-CERT",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret,
						},
						Key: "cert",
					},
				},
			},
			{
				Name: "SMESH-KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret,
						},
						Key: "key",
					},
				},
			},
		},
	}
	return c
}

// func withDebugContainer(pod *corev1.Pod) *corev1.Pod {
// 	privileged := true
// 	secret := pod.Name + "-smesh"
// 	policy := corev1.ContainerRestartPolicyAlways

// 	ec := &corev1.EphemeralContainer{
// 		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
// 			Name:  "smesh-proxy",
// 			Image: "thebsdbox/smesh-proxy:v1",
// 			SecurityContext: &corev1.SecurityContext{
// 				Privileged: &privileged, // TODO: Fix permissions
// 			},
// 			RestartPolicy: &policy,
// 			Env: []corev1.EnvVar{
// 				{
// 					Name: "SMESH-CA",
// 					ValueFrom: &corev1.EnvVarSource{
// 						SecretKeyRef: &corev1.SecretKeySelector{
// 							LocalObjectReference: corev1.LocalObjectReference{
// 								Name: secret,
// 							},
// 							Key: "ca",
// 						},
// 					},
// 				},
// 				{
// 					Name: "SMESH-CERT",
// 					ValueFrom: &corev1.EnvVarSource{
// 						SecretKeyRef: &corev1.SecretKeySelector{
// 							LocalObjectReference: corev1.LocalObjectReference{
// 								Name: secret,
// 							},
// 							Key: "cert",
// 						},
// 					},
// 				},
// 				{
// 					Name: "SMESH-KEY",
// 					ValueFrom: &corev1.EnvVarSource{
// 						SecretKeyRef: &corev1.SecretKeySelector{
// 							LocalObjectReference: corev1.LocalObjectReference{
// 								Name: secret,
// 							},
// 							Key: "key",
// 						},
// 					},
// 				},
// 			},
// 		},
// 		TargetContainerName: "app",
// 	}

// 	copied := pod.DeepCopy()
// 	copied.Spec.EphemeralContainers = append(copied.Spec.EphemeralContainers, *ec)

// 	return copied
// }
