package controller

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	_ reconcile.Reconciler = &Pod200Reconciler{}

	defaultTransport http.RoundTripper = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}

	OKOnlyReadinessGateName = v1.PodConditionType("kubernetes-router.tsuru.io/probe-200-only")
)

type Pod200Reconciler struct {
	client.Client
	Log logr.Logger
}

func (r *Pod200Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	pod := &v1.Pod{}
	if err := r.Get(ctx, request.NamespacedName, pod); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if pod.Spec.ReadinessGates == nil || alreadyMarked(pod) || !belongsToThisController(pod) {
		return reconcile.Result{}, nil
	}

	newCondition := &v1.PodCondition{
		Type:               OKOnlyReadinessGateName,
		Status:             v1.ConditionFalse,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}

	err := r.checkPod(ctx, pod)
	if err == nil {
		newCondition.Status = v1.ConditionTrue
	} else {
		newCondition.Status = v1.ConditionFalse
		newCondition.Reason = "Failed"
		newCondition.Message = err.Error()
	}

	updatePodCondition(&pod.Status, newCondition)

	err = r.Client.Status().Update(ctx, pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *Pod200Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 100,
		}).
		Complete(r)
}

func alreadyMarked(pod *v1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == OKOnlyReadinessGateName {
			if cond.Status == v1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

func belongsToThisController(pod *v1.Pod) bool {
	for _, readinessGate := range pod.Spec.ReadinessGates {
		if readinessGate.ConditionType == OKOnlyReadinessGateName {
			return true
		}
	}

	return false
}

func (r *Pod200Reconciler) checkPod(ctx context.Context, pod *v1.Pod) error {
	containerPort := 8888
	if len(pod.Spec.Containers[0].Ports) > 0 {
		containerPort = int(pod.Spec.Containers[0].Ports[0].ContainerPort)
	}
	probe := pod.Spec.Containers[0].ReadinessProbe
	if probe == nil {
		probe = pod.Spec.Containers[0].LivenessProbe
	}
	path := "/"
	probeTimeout := 30 * time.Second
	scheme := "http"

	if probe != nil {
		probeTimeout = time.Duration(probe.TimeoutSeconds) * time.Second
		if probe.HTTPGet != nil {
			path = probe.HTTPGet.Path

			if probe.HTTPGet.Scheme == v1.URISchemeHTTPS {
				scheme = "https"
			}
		}
	}
	client := http.Client{
		Transport: defaultTransport,
		Timeout:   probeTimeout,
	}

	response, err := client.Get(scheme + "://" + pod.Status.PodIP + ":" + strconv.Itoa(containerPort) + path)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.Errorf("unexpected status code %d", response.StatusCode)
	}
	return nil
}

func updatePodCondition(status *v1.PodStatus, condition *v1.PodCondition) {
	conditionIndex, oldCondition := getPodCondition(status, condition.Type)

	if oldCondition == nil {
		status.Conditions = append(status.Conditions, *condition)
		return
	}

	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	status.Conditions[conditionIndex] = *condition
}

func getPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil || status.Conditions == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}
