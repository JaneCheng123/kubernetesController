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
	"time"

	scanv1 "github.com/JaneCheng123/kubernetesController/api/v1"

	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ClusterScanReconciler reconciles a ClusterScan object
type ClusterScanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ClusterScan object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.3/pkg/reconcile
func (r *ClusterScanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	var clusterScan scanv1.ClusterScan
	err := r.Get(ctx, req.NamespacedName, &clusterScan)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Determine whether to create a Job or a CronJob based on the schedule field
	if clusterScan.Spec.Schedule == "" {
		job := constructJob(&clusterScan)
		err := r.Create(ctx, &job)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		cronJob := constructCronJob(&clusterScan)
		err := r.Create(ctx, &cronJob)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status with the time of the last schedule
	now := metav1.Now()
	clusterScan.Status.LastScheduledTime = now
	err = r.Status().Update(ctx, &clusterScan)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Hour * 1}, nil
}

func constructJob(cs *scanv1.ClusterScan) batchv1.Job {
	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cs.Name + "-job-",
			Namespace:    cs.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: cs.Spec.JobTemplate,
			},
		},
	}
}

func constructCronJob(cs *scanv1.ClusterScan) batchv1beta1.CronJob {
	return batchv1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cs.Name + "-cronjob-",
			Namespace:    cs.Namespace,
		},
		Spec: batchv1beta1.CronJobSpec{
			Schedule: cs.Spec.Schedule,
			JobTemplate: batchv1beta1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: cs.Spec.JobTemplate,
					},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&scanv1.ClusterScan{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1beta1.CronJob{}).
		Complete(r)
}
