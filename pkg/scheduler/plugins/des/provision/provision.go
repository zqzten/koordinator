/*
Copyright 2020 The Kubernetes Authors.

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

package provision

import (
	"context"
	"fmt"

	invsdk "gitlab.alibaba-inc.com/dbpaas/Inventory/inventory-sdk-go/sdk"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	cni2 "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/des/provision/cni"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/des/provision/options"
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "ResourceProvision"
	// reserveENIStateKey is the key in CycleState to Provision reserve eni data.
	reserveENIStateKey = "reserveENI" + Name
)

// reserveENIState computed at reserve and used at unreserve.
type reserveENIState struct {
	networkInterfaceId []string
}

// reserveDiskState computed at reserve and used at unreserve.
type reserveDiskState struct {
	diskIds []string
}

// Clone the reserve state.
func (s *reserveENIState) Clone() framework.StateData {
	return s
}

// Clone the reserve state.
func (s *reserveDiskState) Clone() framework.StateData {
	return s
}

// Provision is a plugin that implements resource provisions prior to binding.
type Provision struct {
	NetworkInterfaceProvisioner cni2.ReserveNetworkInterfaceProvisioner
	NodeLister                  corelisters.NodeLister
	Status                      *framework.Status
}

var _ = framework.ReservePlugin(&Provision{})
var _ = framework.PreBindPlugin(&Provision{})

// Name returns name of the plugin.
func (p *Provision) Name() string {
	return Name
}

func (p *Provision) doCNI(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) error {
	// if not db engine, return
	if _, exist := pod.Labels[options.InstanceEngineLabel]; !exist {
		return nil
	}

	// if not need to customize eni, return
	if pod.Labels[options.UseENILabel] != "true" && pod.Labels[options.UseTrunkENILabel] != "true" {
		return nil
	}

	// if hostNetwork, return
	if pod.Spec.HostNetwork == true {
		return nil
	}

	node, err := p.NodeLister.Get(nodeName)
	if err != nil {
		return fmt.Errorf("get node %s failed: %s", nodeName, err.Error())
	}

	// Step1: create eni
	interfaceSets, err := p.NetworkInterfaceProvisioner.CreateEni(pod, node)
	if err != nil {
		return fmt.Errorf("create eni failed: %s", err.Error())
	}

	var enis []string
	for _, ifs := range interfaceSets {
		enis = append(enis, ifs.NetworkInterfaceId)
	}
	// Step2: write eni data to CycleState
	state.Write(reserveENIStateKey, &reserveENIState{networkInterfaceId: enis})

	// Step3: patch eni data to pod
	err = p.NetworkInterfaceProvisioner.PatchEni(ctx, pod, node, interfaceSets)
	return err
}

func (p *Provision) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {

	return nil
}

// Unreserve do nothing
func (p *Provision) Unreserve(ctx context.Context, cs *framework.CycleState, pod *corev1.Pod, nodeName string) {
	// if not db engine, return
	if _, exist := pod.Labels[options.InstanceEngineLabel]; !exist {
		return
	}

	// if not need to customize eni, return
	if pod.Labels[options.UseENILabel] != "true" && pod.Labels[options.UseTrunkENILabel] != "true" {
		return
	}

	// if hostNetwork, return
	if pod.Spec.HostNetwork == true {
		return
	}

	// if pod use eci, return directly
	if pod.Labels[options.EnableECILabel] == "true" {
		return
	}

	klog.Infof("Unreserve pod %s", pod.Name)
	// Step1: reserve eni
	s, err := getReserveENIState(cs)
	if err != nil {
		klog.Errorf("pod %v getReserveENIState error: %v", klog.KObj(pod), err)
		return
	}
	for _, eni := range s.networkInterfaceId {
		err = p.NetworkInterfaceProvisioner.DeleteEni(ctx, pod, eni)
		if err != nil {
			klog.Errorf("pod %v deleteEni %s error: %v", klog.KObj(pod), s.networkInterfaceId, err)
		}
	}

	return
}

func (p *Provision) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	// if pod use eci, return directly
	if pod.Labels[options.EnableECILabel] == "true" {
		return p.Status
	}

	// PreBind: Do process CNI
	err := p.doCNI(ctx, cycleState, pod, nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("DoCNI failed for pod %s in node %s: %s", pod.Name, nodeName, err.Error()))
	}

	klog.Infof("Provision pod[%s] eni in node %s prebind done!!!", pod.Name, nodeName)
	return p.Status
}

func getReserveENIState(cycleState *framework.CycleState) (*reserveENIState, error) {
	c, err := cycleState.Read(reserveENIStateKey)
	if err != nil {
		// getReserveENIState doesn't exist, likely reserve wasn't invoked.
		return nil, fmt.Errorf("error reading %q from cycleState: %v", reserveENIStateKey, err)
	}

	s, ok := c.(*reserveENIState)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to Provision.reserveENIState error", c)
	}
	return s, nil
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, fh framework.Handle) (framework.Plugin, error) {
	klog.InfoS("Resource provision plugin start")

	podInformer := fh.SharedInformerFactory().Core().V1().Pods()
	nodeInformer := fh.SharedInformerFactory().Core().V1().Nodes()
	pvcInformer := fh.SharedInformerFactory().Core().V1().PersistentVolumeClaims()
	pvInformer := fh.SharedInformerFactory().Core().V1().PersistentVolumes()
	storageClassInformer := fh.SharedInformerFactory().Storage().V1().StorageClasses()
	kubeClient := fh.ClientSet()
	invClient, err := invsdk.NewNativeClient()
	if err != nil {
		return nil, err
	}
	accountMap, err := kubeClient.CoreV1().ConfigMaps("kube-system").Get(context.Background(), options.InventoryAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	networkInterfaceProvisioner := cni2.NewNetworkInterfaceProvisioner(kubeClient, podInformer, nodeInformer, pvcInformer, pvInformer, storageClassInformer, invClient, accountMap)
	return &Provision{
		NetworkInterfaceProvisioner: networkInterfaceProvisioner,
		NodeLister:                  nodeInformer.Lister(),
		Status:                      nil,
	}, nil
}
