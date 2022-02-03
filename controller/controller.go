package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &Pod200Reconciler{}

type Pod200Reconciler struct {
	client.Client
	Log logr.Logger
}

func (r *Pod200Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	fmt.Println(request)
	return reconcile.Result{}, nil
}

func (r *Pod200Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		Complete(r)
}
