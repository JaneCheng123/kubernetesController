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
	"fmt"
	"time"

	"github.com/robfig/cron"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	scanv1 "github.com/JaneCheng123/kubernetesController/api/v1"
)

// ClusterScanReconciler reconciles a ClusterScan object
type ClusterScanReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Clock  clock.Clock
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
	log := log.FromContext(ctx)

	// Load the named ClusterScan
	var clusterScan scanv1.ClusterScan
	err := r.Get(ctx, req.NamespacedName, &clusterScan)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if suspended
	if clusterScan.Spec.Suspend != nil && *clusterScan.Spec.Suspend {
		log.Info("ClusterScan is suspended, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// List all active jobs, and update the status
	var activeJobs batchv1.JobList
	err = r.List(ctx, &activeJobs, client.InNamespace(req.Namespace), client.MatchingFields{"metadata.ownerReferences.uid": string(clusterScan.UID)})
	if err != nil {
		return ctrl.Result{}, err
	}
	activeJobCount := len(activeJobs.Items)
	if activeJobCount > 0 {
		clusterScan.Status.JobStatus = "Active"
	} else {
		clusterScan.Status.JobStatus = "Completed"
	}

	err = r.Status().Update(ctx, &clusterScan)
	if err != nil {
		return ctrl.Result{}, err
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
	clusterScan.Status.LastScheduleTime = &now
	err = r.Status().Update(ctx, &clusterScan)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Requeue when we either see a running job or it’s time for the next scheduled run
	nextRun := r.getNextSchedule(&clusterScan)
	if nextRun.IsZero() {
		log.Info("No more scheduled runs, skipping")
		return ctrl.Result{}, nil
	}
	return ctrl.Result{RequeueAfter: nextRun.Sub(time.Now())}, nil
}

func constructJob(clusterScan *scanv1.ClusterScan) batchv1.Job {
	var command []string
	switch clusterScan.Spec.SecurityTool {
	case "Kubevious":
		command = []string{"kubevious-cli", "scan", "--kubeconfig", "/root/.kube/config"}
	case "TerraScan":
		command = []string{"terrascan", "scan", "--iac-type", "k8s", "--config-path", "/root/.kube/config"}
	default:
		command = []string{"sh", "-c", fmt.Sprintf("echo Running %s", clusterScan.Spec.SecurityTool)}
	}

	return batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: clusterScan.Name + "-job-",
			Namespace:    clusterScan.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "security-scan",
						Image:   "busybox",
						Command: command,
					}},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}
}

func constructCronJob(clusterScan *scanv1.ClusterScan) batchv1.CronJob {
	return batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: clusterScan.Name + "-cronjob-",
			Namespace:    clusterScan.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: clusterScan.Spec.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: clusterScan.Spec.JobTemplate,
					},
				},
			},
		},
	}
}

func (r *ClusterScanReconciler) getNextSchedule(clusterScan *scanv1.ClusterScan) time.Time {
	sched, err := cron.ParseStandard(clusterScan.Spec.Schedule)
	if err != nil {
		return time.Time{}
	}
	return sched.Next(time.Now())
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterScanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create an index for the ownerReferences field
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &batchv1.Job{}, "metadata.ownerReferences.uid", func(rawObj client.Object) []string {
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != scanv1.GroupVersion.String() || owner.Kind != "ClusterScan" {
			return nil
		}
		return []string{string(owner.UID)}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&scanv1.ClusterScan{}).
		Complete(r)
}
