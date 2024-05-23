/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	scanv1 "github.com/JaneCheng123/kubernetesController/api/v1"
)

func init() {
	_ = scanv1.AddToScheme(scheme.Scheme)
}

var _ = Describe("ClusterScan Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		clusterscan := &scanv1.ClusterScan{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ClusterScan")
			err := k8sClient.Get(ctx, typeNamespacedName, clusterscan)
			if err != nil && errors.IsNotFound(err) {
				resource := &scanv1.ClusterScan{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: scanv1.ClusterScanSpec{
						Schedule: "*/1 * * * *",
						JobTemplate: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:    "busybox",
								Image:   "busybox",
								Command: []string{"sh", "-c", "echo hello"},
							}},
							RestartPolicy: corev1.RestartPolicyOnFailure,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &scanv1.ClusterScan{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ClusterScan")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ClusterScanReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func TestConstructJob(t *testing.T) {
	clusterScan := &scanv1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-clusterscan",
			Namespace: "default",
		},
		Spec: scanv1.ClusterScanSpec{
			JobTemplate: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:    "security-scan",
					Image:   "busybox",
					Command: []string{"sh", "-c", "echo hello"},
				}},
				RestartPolicy: corev1.RestartPolicyOnFailure,
			},
		},
	}

	job := constructJob(clusterScan)
	assert.Equal(t, job.Spec.Template.Spec.Containers[0].Name, "security-scan")
	assert.Equal(t, job.Spec.Template.Spec.Containers[0].Image, "busybox")
	assert.Equal(t, job.Spec.Template.Spec.RestartPolicy, corev1.RestartPolicyOnFailure)
}

func TestConstructCronJob(t *testing.T) {
	clusterScan := &scanv1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-clusterscan",
			Namespace: "default",
		},
		Spec: scanv1.ClusterScanSpec{
			Schedule: "*/1 * * * *",
			JobTemplate: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:    "security-scan",
					Image:   "busybox",
					Command: []string{"sh", "-c", "echo hello"},
				}},
				RestartPolicy: corev1.RestartPolicyOnFailure,
			},
		},
	}

	cronJob := constructCronJob(clusterScan)
	assert.Equal(t, cronJob.Spec.Schedule, "*/1 * * * *")
	assert.Equal(t, cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Name, "security-scan")
	assert.Equal(t, cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image, "busybox")
	assert.Equal(t, cronJob.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy, corev1.RestartPolicyOnFailure)
}

func TestGetNextSchedule(t *testing.T) {
	clusterScan := &scanv1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-clusterscan",
			Namespace: "default",
		},
		Spec: scanv1.ClusterScanSpec{
			Schedule: "*/1 * * * *",
		},
	}

	reconciler := &ClusterScanReconciler{}
	nextRun := reconciler.getNextSchedule(clusterScan)
	assert.NotEqual(t, nextRun, time.Time{})
}
