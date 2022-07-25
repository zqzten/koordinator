package unified

import (
	"context"
	"fmt"
	"path"

	sev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/reservation"
)

const (
	LabelParentReservation                = "alibabacloud.com/parent-rr"
	LabelMigrationWorkflowOriginSN        = "alibabacloud.com/migrationworkflow-origin-podsn"
	LabelMigrationWorkflowOriginNamespace = "alibabacloud.com/migrationworkflow-origin-pod-namespace"
	LabelMigrationConfirmState            = "alibabacloud.com/migration-confirm-state"

	// LabelEnableMigrate represents trigger preemption for ReserveResource
	LabelEnableMigrate = "alibabacloud.com/enable-migrate"
)

func init() {
	reservation.NewInterpreter = NewInterpreter
}

// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=reserveresources,verbs=get;list;watch;create;update;patch;delete

var _ reservation.Interpreter = &Interpreter{}

type Interpreter struct {
	mgr manager.Manager
	client.Client
}

func NewInterpreter(mgr ctrl.Manager) reservation.Interpreter {
	_ = sev1.AddToScheme(mgr.GetScheme())

	return &Interpreter{
		mgr:    mgr,
		Client: mgr.GetClient(),
	}
}

func (p *Interpreter) GetReservationType() client.Object {
	return &sev1.ReserveResource{}
}

func (p *Interpreter) Preemption() reservation.Preemption {
	return p
}

func (p *Interpreter) GetReservation(ctx context.Context, ref *corev1.ObjectReference) (reservation.Object, error) {
	rr := &sev1.ReserveResource{}
	namespacedName := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	err := p.Client.Get(ctx, namespacedName, rr)
	if k8serrors.IsNotFound(err) {
		klog.Warningf("Failed to get ReserveResource %v, reason: %v", namespacedName, err)
		err = p.mgr.GetAPIReader().Get(ctx, namespacedName, rr)
	}
	return &Reservation{
		ReserveResource: rr,
	}, err
}

func (p *Interpreter) DeleteReservation(ctx context.Context, ref *corev1.ObjectReference) error {
	if ref == nil {
		return nil
	}

	id := reservation.GetReservationNamespacedName(ref)
	klog.V(4).Infof("begin to delete ReserveResource: %v", id)

	rrList := &sev1.ReserveResourceList{}
	opts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{LabelParentReservation: ref.Name}),
		// DisableDeepCopy: true,
	}
	err := p.Client.List(ctx, rrList, opts)
	if err == nil {
		for _, rr := range rrList.Items {
			err = p.Client.Delete(ctx, &rr)
			if err != nil {
				klog.Errorf("Failed to delete child ReserveResource %s/%s, err: %v", rr.Namespace, rr.Name, err)
				return err
			}
			klog.V(4).Infof("Successfully delete child ReserveResource %s/%s", rr.Namespace, rr.Name)
		}
	}

	rr, err := p.GetReservation(ctx, ref)
	if err == nil {
		err = p.Client.Delete(ctx, rr.OriginObject())
		if err != nil {
			klog.Errorf("Failed to delete ReserveResource %v, err: %v", id, err)
		} else {
			klog.V(4).Infof("Successfully delete ReserveResource %v", id)
		}
	}
	return err
}

func (p *Interpreter) CreateReservation(ctx context.Context, job *koordsev1alpha1.PodMigrationJob) (reservation.Object, error) {
	reservationOptions := job.Spec.ReservationOptions
	if reservationOptions == nil {
		return nil, fmt.Errorf("invalid reservationOptions")
	}

	resourceOwner := convertResourceOwner(reservationOptions.Template.Spec.Owners)
	if resourceOwner == nil {
		return nil, fmt.Errorf("invalid resourceOwner")
	}

	// TODO(joseph.lt): we should support Patch to merge template
	reserveResource := &sev1.ReserveResource{
		ObjectMeta: reservationOptions.Template.ObjectMeta,
		Spec: sev1.ReserveResourceSpec{
			Template:            reservationOptions.Template.Spec.Template,
			ResourceOwners:      []sev1.ResourceOwner{*resourceOwner},
			Priority:            reservationOptions.Template.Spec.Template.Spec.Priority,
			SchedulerName:       reservationOptions.Template.Spec.Template.Spec.SchedulerName,
			DeleteAfterOwnerUse: true,
		},
	}

	if reservationOptions.Template.Spec.TTL != nil {
		reserveResource.Spec.TimeToLiveDuration = reservationOptions.Template.Spec.TTL.DeepCopy()
	} else if reservationOptions.Template.Spec.Expires != nil {
		reserveResource.Spec.Deadline = reservationOptions.Template.Spec.Expires.DeepCopy()
	}

	if job.Labels[LabelEnableMigrate] == "true" {
		if reserveResource.Labels == nil {
			reserveResource.Labels = make(map[string]string)
		}
		reserveResource.Labels[LabelEnableMigrate] = "true"
		reserveResource.Spec.ConfirmState = sev1.ReserveResourceConfirmState(job.Labels[LabelMigrationConfirmState])
	}

	err := p.Client.Create(ctx, reserveResource)
	if k8serrors.IsAlreadyExists(err) {
		err = p.Client.Get(ctx, types.NamespacedName{Namespace: reserveResource.Namespace, Name: reserveResource.Name}, reserveResource)
	}
	if err != nil {
		return nil, err
	}
	return &Reservation{ReserveResource: reserveResource}, nil
}

func convertResourceOwner(owners []koordsev1alpha1.ReservationOwner) *sev1.ResourceOwner {
	for _, v := range owners {
		if v.Object != nil {
			return &sev1.ResourceOwner{
				AllocMeta: &sev1.AllocMeta{
					UID:       v.Object.UID,
					Kind:      v.Object.Kind,
					Name:      v.Object.Name,
					Namespace: v.Object.Namespace,
				},
			}
		} else if v.Controller != nil {
			return &sev1.ResourceOwner{
				ControllerKey: path.Join(v.Controller.Namespace, v.Controller.Name, v.Controller.Kind),
			}
		}
	}
	return nil
}

func (p *Interpreter) queryChildReservations(ctx context.Context, reservationRef *corev1.ObjectReference) ([]koordsev1alpha1.PodMigrationJobPreemptedReservation, error) {
	rrList := &sev1.ReserveResourceList{}
	listOptions := client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(labels.Set{LabelParentReservation: reservationRef.Name})}
	err := p.Client.List(ctx, rrList, listOptions)
	if err != nil {
		return nil, err
	}
	var childRRs []koordsev1alpha1.PodMigrationJobPreemptedReservation
	for _, childRR := range rrList.Items {
		var podsRef []corev1.ObjectReference
		for _, v := range childRR.Status.Allocs {
			podsRef = append(podsRef, corev1.ObjectReference{
				Namespace: v.Namespace,
				Name:      v.Name,
				Kind:      v.Kind,
				UID:       v.UID,
			})
		}
		childRRs = append(childRRs, koordsev1alpha1.PodMigrationJobPreemptedReservation{
			Namespace: childRR.Namespace,
			Name:      childRR.Name,
			NodeName:  childRR.Spec.NodeName,
			Phase:     string(childRR.Status.State),
			PreemptedPodRef: &corev1.ObjectReference{
				Namespace: childRR.Labels[LabelMigrationWorkflowOriginNamespace],
				Name:      childRR.Labels[LabelMigrationWorkflowOriginSN],
			},
			PodsRef: podsRef,
		})
	}
	return childRRs, nil
}
