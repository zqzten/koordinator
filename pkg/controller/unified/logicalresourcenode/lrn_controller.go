/*
Copyright 2022 The Koordinator Authors.

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

package logicalresourcenode

import (
	"context"
	"fmt"

	terwayapis "github.com/AliyunContainerService/terway-apis/network.alibabacloud.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"
)

// LogicalResourceNodeReconciler reconciles a LogicalResourceNode object
type LogicalResourceNodeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	reservationReconciler internalReconciler
}

type internalReconciler interface {
	reconcile(ctx context.Context, cfg *lrnutil.Config, lrn *schedulingv1alpha1.LogicalResourceNode) (ctrl.Result, error)
}

// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=logicalresourcenodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=logicalresourcenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.koordinator.sh,resources=reservations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.alibabacloud.com,resources=eniqosgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch;delete

func (r *LogicalResourceNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	klog.Infof("Start to reconcile LogicalResourceNode %s", req.Name)
	defer func() {
		if retErr != nil {
			klog.Infof("Finished to reconcile LogicalResourceNode %s, err: %s", req.Name, retErr)
			return
		}
		klog.Infof("Finished to reconcile LogicalResourceNode %s", req.Name)
	}()

	var err error
	lrn := &schedulingv1alpha1.LogicalResourceNode{}
	if err = r.Get(ctx, req.NamespacedName, lrn); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if satisfied, duration := lrnExpectations.IsSatisfied(lrn); !satisfied {
		return ctrl.Result{}, fmt.Errorf("unsatisfied LRN for %s duration", duration)
	}

	cfg, err := lrnutil.GetConfig(r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get lrn config: %v", err)
	}

	return r.reservationReconciler.reconcile(ctx, cfg, lrn)
}

func Add(mgr ctrl.Manager) error {
	cfg, err := lrnutil.GetConfig(mgr.GetAPIReader())
	if err != nil {
		return fmt.Errorf("failed to get lrn config when setup: %v", err)
	}
	klog.Infof("Initializing lrn controller with config: %v", util.DumpJSON(cfg))

	if err := mgr.GetCache().IndexField(context.Background(), &schedulingv1alpha1.LogicalResourceNode{}, "status.nodeName", func(obj client.Object) []string {
		lrn, ok := obj.(*schedulingv1alpha1.LogicalResourceNode)
		if !ok {
			return nil
		}
		if len(lrn.Status.NodeName) == 0 {
			return nil
		}
		return []string{lrn.Status.NodeName}
	}); err != nil {
		return err
	}
	if err := mgr.GetCache().IndexField(context.Background(), &schedulingv1alpha1.Reservation{}, "ownerRefLRN", func(obj client.Object) []string {
		reservation, ok := obj.(*schedulingv1alpha1.Reservation)
		if !ok {
			return []string{}
		}
		if name := lrnutil.GetReservationOwnerLRN(reservation); name != "" {
			return []string{name}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := mgr.GetCache().IndexField(context.Background(), &corev1.Pod{}, "reservationAllocatedUID", func(obj client.Object) (vals []string) {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		ra, _ := apiext.GetReservationAllocated(pod)
		if ra != nil && len(ra.UID) > 0 {
			return []string{string(ra.UID)}
		}
		return nil
	}); err != nil {
		return err
	}

	reconciler := LogicalResourceNodeReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Recorder:              mgr.GetEventRecorderFor("logicalresourcenode-controller"),
		reservationReconciler: &reservationReconciler{Client: mgr.GetClient()},
	}
	return reconciler.setup(mgr, cfg)
}

func (r *LogicalResourceNodeReconciler) setup(mgr ctrl.Manager, cfg *lrnutil.Config) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&schedulingv1alpha1.LogicalResourceNode{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: *workerNumFlag}).
		Watches(&schedulingv1alpha1.Reservation{}, handler.EnqueueRequestForOwner(
			mgr.GetScheme(), mgr.GetRESTMapper(), &schedulingv1alpha1.LogicalResourceNode{}, handler.OnlyControllerOwner())).
		Watches(&corev1.Node{}, &nodeEventHandler{cache: mgr.GetCache()}).
		Named("logicalresourcenode")
	if cfg.Common.EnableQoSGroup {
		builder.Watches(&terwayapis.ENIQosGroup{}, &qosGroupEventHandler{})
	}
	return builder.Complete(r)
}
