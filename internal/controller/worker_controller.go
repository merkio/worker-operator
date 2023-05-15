/*
Copyright 2023.

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

	_ "container/list"

	"github.com/go-logr/logr"
	"github.com/merkio/worker-operator/internal/kafka"
	"github.com/merkio/worker-operator/internal/utils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkerStatus string

const (
	WorkerStarted    WorkerStatus = "Started"
	WorkerTerminated WorkerStatus = "Terminated"
)

// PodReconciler reconciles a Worker object
type PodReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Log              logr.Logger
	MessagePublisher *kafka.KafkaProducer
}

//+kubebuilder:rbac:groups=k8s.space.geek,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.space.geek,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.space.geek,resources=pods/finalizers,verbs=update

const (
	selectorLabel  = "app"
	selectorValue  = "worker"
	finalizerValue = "kubernetes"
	podNameLabel   = "podName"
	podIpLabel     = "podIp"
)

func (r *PodReconciler) IsValid(obj metav1.Object) bool {
	return obj.GetLabels()[selectorLabel] == selectorValue
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Worker object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pod", req.NamespacedName)
	log.Info(fmt.Sprintf("Reconcile request: %v", req))
	/*
		Step 0: Fetch the Pod from the Kubernetes API.
	*/

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}
	if r.IsValid(pod.GetObjectMeta()) {
		status := pod.Status
		if utils.IsBeingDeleted(&pod) {
			if utils.HasFinalizer(&pod, finalizerValue) {
				log.Info("Remove finalizer and publish message")
				r.sendMessage(pod, WorkerTerminated)
				utils.RemoveFinalizer(&pod, finalizerValue)
				r.updatePod(ctx, pod, log)
			}
			return ctrl.Result{}, nil
		}
		if status.Phase == corev1.PodFailed || status.Phase == corev1.PodUnknown {
			log.Info(fmt.Sprintf("Pod failed: %v - %v", status.Phase, status.Message))
			return ctrl.Result{}, nil
		}

		if status.Phase == corev1.PodRunning {
			if podIp := pod.Labels[podIpLabel]; podIp != "" {
				r.sendMessage(pod, WorkerStarted)
			}
			r.setLabels(ctx, pod, log)
		}
	}

	return ctrl.Result{}, nil
}

func (r *PodReconciler) setLabels(ctx context.Context, pod corev1.Pod, log logr.Logger) bool {
	if !utils.HasFinalizer(&pod, finalizerValue) {
		utils.AddFinalizer(&pod, finalizerValue)
	}
	if pod.Labels != nil {
		podNameL := pod.Labels[podNameLabel]
		podIpL := pod.Labels[podIpLabel]

		status := pod.Status
		if podNameL == "" {
			r.setLabel(ctx, pod, log, podNameLabel, pod.Name)
		}
		if podIpL == "" && status.Phase == corev1.PodRunning {
			r.setLabel(ctx, pod, log, podIpLabel, status.PodIP)
		}
		return true
	}
	return false
}

func (r *PodReconciler) setLabel(ctx context.Context, pod corev1.Pod, log logr.Logger, label string, value string) {
	pod.Labels[label] = value
	log.Info(fmt.Sprintf("New labels: %v", pod.Labels))
	r.updatePod(ctx, pod, log)
}

func (r *PodReconciler) sendMessage(pod corev1.Pod, status WorkerStatus) {
	message := kafka.Message{
		Status:   string(status),
		WorkerID: pod.Name,
		WorkerIP: pod.Status.PodIP,
	}
	r.MessagePublisher.PublishMessage(message)
}

func (r *PodReconciler) updatePod(ctx context.Context, pod corev1.Pod, log logr.Logger) {
	if err := r.Update(ctx, &pod); err != nil {
		if apierrors.IsConflict(err) {
			// The Pod has been updated since we read it.
			// Requeue the Pod to try to reconciliate again.
			log.Error(err, "Pod has been updated since we read it")
		}
		if apierrors.IsNotFound(err) {
			// The Pod has been deleted since we read it.
			// Requeue the Pod to try to reconciliate again.
			log.Error(err, "Pod has been deleted since we read it")
		}
		log.Error(err, "unable to update Pod")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
