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

package eci

import (
	"context"
	"fmt"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

var (
	_ framework.FilterPlugin  = &Plugin{}
	_ framework.ScorePlugin   = &Plugin{}
	_ framework.PreBindPlugin = &Plugin{}
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedECI"

	ErrReasonCannotRunOnECI = "node(s) can't run pod which can't run on eci"
	ErrReasonMustRunOnECI   = "node(s) can't run pod which must run on eci"
)

type Plugin struct {
	handle framework.Handle
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Plugin{
		handle: handle,
	}, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	podECIAffinity := pod.Labels[uniext.LabelECIAffinity]
	switch podECIAffinity {
	case uniext.ECIRequired:
		if extunified.IsVirtualKubeletNode(node) {
			return nil
		}
		return framework.NewStatus(framework.Unschedulable, ErrReasonMustRunOnECI)
	case uniext.ECIRequiredNot:
		if !extunified.IsVirtualKubeletNode(node) {
			return nil
		}
		return framework.NewStatus(framework.Unschedulable, ErrReasonCannotRunOnECI)
	default:
		return nil
	}
}

func (p *Plugin) Score(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found")
	}
	score := int64(0)
	if extunified.IsVirtualKubeletNode(node) {
		score = ScoreVKNode(pod.Labels)
	} else {
		score = ScoreNormalNode()
	}
	return score, nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if !extunified.IsVirtualKubeletNode(node) {
		return nil
	}

	podOriginal := pod
	pod = pod.DeepCopy()
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[uniext.LabelCommonNodeType] = uniext.VKType

	patchBytes, err := util.GeneratePodPatch(podOriginal, pod)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	if string(patchBytes) == "{}" {
		return nil
	}
	err = retry.OnError(
		retry.DefaultRetry,
		errors.IsTooManyRequests,
		func() error {
			_, err := p.handle.ClientSet().CoreV1().Pods(pod.Namespace).
				Patch(ctx, pod.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			if err != nil {
				klog.Error("Failed to patch Pod %s/%s, patch: %v, err: %v", pod.Namespace, pod.Name, string(patchBytes), err)
			}
			return err
		})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	klog.V(4).Infof("Successfully mark Pod %s/%s as vk", pod.Namespace, pod.Name)
	return nil
}
