package scheduling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgov1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
	k8spodutil "k8s.io/kubernetes/pkg/api/v1/pod"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
	"github.com/koordinator-sh/koordinator/test/e2e/framework"
	"github.com/koordinator-sh/koordinator/test/e2e/framework/manifest"
)

var _ = SIGDescribe("InplaceUpdate", func() {
	f := framework.NewDefaultFramework("inplace-update")

	framework.KoordinatorDescribe("Normal Inplace Update Flow", func() {
		framework.ConformanceIt("Create Pod, initiate two inplaceUpdate Request to scale up cpu request and scale down cpu request", func() {
			ginkgo.By("Loading Pod from manifest")
			pod, err := manifest.PodFromManifest("scheduling/simple-lsr-pod.yaml")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Setting original container resource")
			originalContainerResource := pod.Spec.Containers[0].Resources
			originalContainerResource.Requests[corev1.ResourceCPU] = resource.MustParse("2")
			originalContainerResource.Limits[corev1.ResourceCPU] = resource.MustParse("2")
			pod.Spec.Containers[0].Resources = originalContainerResource

			ginkgo.By("Create Pod")
			pod.Namespace = f.Namespace.Name
			pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			})
			pod = f.PodClient().Create(pod)

			ginkgo.By("Wait for Pod Scheduled")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				_, podCondition := k8spodutil.GetPodCondition(&p.Status, corev1.PodScheduled)
				return podCondition != nil && podCondition.Status == corev1.ConditionTrue && p.Status.Phase == corev1.PodRunning
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check Pod ResourceStatus")
			pod, err = f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			oldResourceStatus, err := extension.GetResourceStatus(pod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(oldResourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err := cpuset.Parse(oldResourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(2))

			ginkgo.By("Initiate First InplaceUpdate Request, cpu request update from 2 to 3")
			updatedContainerResource := pod.Spec.Containers[0].Resources
			updatedContainerResource.Requests[corev1.ResourceCPU] = resource.MustParse("3")
			updatedContainerResource.Limits[corev1.ResourceCPU] = resource.MustParse("3")
			resourceUpdateSpec := &uniext.ResourceUpdateSpec{
				Version: "1",
				Containers: []uniext.ContainerResource{
					{
						Name:      pod.Spec.Containers[0].Name,
						Resources: updatedContainerResource,
					},
				},
			}

			f.PodClient().Update(pod.Name, func(pod *corev1.Pod) {
				resourceUpdateSpecData, err := json.Marshal(resourceUpdateSpec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				pod.Annotations[uniext.AnnotationResourceUpdateSpec] = string(resourceUpdateSpecData)
			})

			ginkgo.By("Wait for Pod InplaceUpdate Complete")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return !podHasUnhandledUpdateReq(p)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check InplaceUpdate Scheduler State")
			newPod, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			schedulerState, err := uniext.GetResourceUpdateSchedulerState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(schedulerState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(schedulerState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateAccepted))

			ginkgo.By("Check InplaceUpdate Kubelet State")
			kubeletState, err := uniext.GetResourceUpdateNodeState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(kubeletState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(kubeletState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateSucceeded))

			ginkgo.By("Check Container Request and Limit")
			gomega.Expect(newPod.Spec.Containers[0].Resources).Should(gomega.Equal(updatedContainerResource))

			ginkgo.By("Check CPUSet update status")
			resourceStatus, err := extension.GetResourceStatus(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err = cpuset.Parse(resourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(3))

			ginkgo.By("Initiate Second InplaceUpdate Request, cpu request update from 3 to 1")
			updatedContainerResource = newPod.Spec.Containers[0].Resources
			updatedContainerResource.Requests[corev1.ResourceCPU] = resource.MustParse("1")
			updatedContainerResource.Limits[corev1.ResourceCPU] = resource.MustParse("1")
			resourceUpdateSpec = &uniext.ResourceUpdateSpec{
				Version: "2",
				Containers: []uniext.ContainerResource{
					{
						Name:      pod.Spec.Containers[0].Name,
						Resources: updatedContainerResource,
					},
				},
			}

			f.PodClient().Update(pod.Name, func(pod *corev1.Pod) {
				resourceUpdateSpecData, err := json.Marshal(resourceUpdateSpec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				pod.Annotations[uniext.AnnotationResourceUpdateSpec] = string(resourceUpdateSpecData)
			})

			ginkgo.By("Wait for Pod InplaceUpdate Complete")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return !podHasUnhandledUpdateReq(p)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check InplaceUpdate Scheduler State")
			newPod, err = f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			schedulerState, err = uniext.GetResourceUpdateSchedulerState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(schedulerState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(schedulerState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateAccepted))

			ginkgo.By("Check InplaceUpdate Kubelet State")
			kubeletState, err = uniext.GetResourceUpdateNodeState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(kubeletState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(kubeletState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateSucceeded))

			ginkgo.By("Check Container Request and Limit")
			gomega.Expect(newPod.Spec.Containers[0].Resources).Should(gomega.Equal(updatedContainerResource))

			ginkgo.By("Check CPUSet update status")
			resourceStatus, err = extension.GetResourceStatus(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err = cpuset.Parse(resourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(1))
		})

		framework.ConformanceIt("When node resource fully utilized, Create Pod, initiate three inplaceUpdate Request, one to use the resource, one to scale up and the other to scale down", func() {
			ginkgo.By("Loading Pod from manifest")
			pod, err := manifest.PodFromManifest("scheduling/simple-lsr-pod.yaml")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Setting original container resource")
			originalContainerResource := pod.Spec.Containers[0].Resources
			originalContainerResource.Requests[corev1.ResourceCPU] = resource.MustParse("2")
			originalContainerResource.Limits[corev1.ResourceCPU] = resource.MustParse("2")
			pod.Spec.Containers[0].Resources = originalContainerResource

			ginkgo.By("Create Pod")
			pod.Namespace = f.Namespace.Name
			pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			})
			pod = f.PodClient().Create(pod)

			ginkgo.By("Wait for Pod Scheduled")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				_, podCondition := k8spodutil.GetPodCondition(&p.Status, corev1.PodScheduled)
				return podCondition != nil && podCondition.Status == corev1.ConditionTrue && p.Status.Phase == corev1.PodRunning
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check Pod ResourceStatus")
			pod, err = f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			oldResourceStatus, err := extension.GetResourceStatus(pod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(oldResourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err := cpuset.Parse(oldResourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(2))

			ginkgo.By("Update Node With New Resource")
			newResourceName := corev1.ResourceName("foo/bar")
			nodeClient := f.ClientSet.CoreV1().Nodes()
			updateNode(nodeClient, pod.Spec.NodeName, func(node *corev1.Node) {
				node.Status.Allocatable[newResourceName] = resource.MustParse("10")
				node.Status.Capacity[newResourceName] = resource.MustParse("10")
			})
			defer func() {
				ginkgo.By("clear node NewResource")
				updateNode(nodeClient, pod.Spec.NodeName, func(node *corev1.Node) {
					delete(node.Status.Allocatable, newResourceName)
					delete(node.Status.Capacity, newResourceName)
				})
			}()

			ginkgo.By("Initiate First InplaceUpdate Request, foo/bar request update from 0 to 8")
			updatedContainerResource := pod.Spec.Containers[0].Resources
			updatedContainerResource.Requests[newResourceName] = resource.MustParse("8")
			updatedContainerResource.Limits[newResourceName] = resource.MustParse("8")
			resourceUpdateSpec := &uniext.ResourceUpdateSpec{
				Version: "1",
				Containers: []uniext.ContainerResource{
					{
						Name:      pod.Spec.Containers[0].Name,
						Resources: updatedContainerResource,
					},
				},
			}
			f.PodClient().Update(pod.Name, func(pod *corev1.Pod) {
				resourceUpdateSpecData, err := json.Marshal(resourceUpdateSpec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				pod.Annotations[uniext.AnnotationResourceUpdateSpec] = string(resourceUpdateSpecData)
			})

			ginkgo.By("Wait for Pod InplaceUpdate Complete")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return !podHasUnhandledUpdateReq(p)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check InplaceUpdate Scheduler State")
			newPod, err := f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			schedulerState, err := uniext.GetResourceUpdateSchedulerState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(schedulerState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(schedulerState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateAccepted))

			ginkgo.By("Check InplaceUpdate Kubelet State")
			kubeletState, err := uniext.GetResourceUpdateNodeState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(kubeletState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(kubeletState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateSucceeded))

			ginkgo.By("Check Container Request and Limit")
			gomega.Expect(newPod.Spec.Containers[0].Resources).Should(gomega.Equal(updatedContainerResource))

			ginkgo.By("Check CPUSet update status")
			resourceStatus, err := extension.GetResourceStatus(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err = cpuset.Parse(resourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(2))

			ginkgo.By("Initiate Second InplaceUpdate Request, foo/bar request update from 8 to 11")
			updatedContainerResource = newPod.Spec.Containers[0].Resources
			updatedContainerResource.Requests[newResourceName] = resource.MustParse("11")
			updatedContainerResource.Limits[newResourceName] = resource.MustParse("11")
			resourceUpdateSpec = &uniext.ResourceUpdateSpec{
				Version: "2",
				Containers: []uniext.ContainerResource{
					{
						Name:      pod.Spec.Containers[0].Name,
						Resources: updatedContainerResource,
					},
				},
			}

			f.PodClient().Update(pod.Name, func(pod *corev1.Pod) {
				resourceUpdateSpecData, err := json.Marshal(resourceUpdateSpec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				pod.Annotations[uniext.AnnotationResourceUpdateSpec] = string(resourceUpdateSpecData)
			})

			ginkgo.By("Wait for Pod InplaceUpdate Complete")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return !podHasUnhandledUpdateReq(p)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check InplaceUpdate Scheduler State")
			newPod, err = f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			schedulerState, err = uniext.GetResourceUpdateSchedulerState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(schedulerState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(schedulerState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateRejected))

			ginkgo.By("Check InplaceUpdate Kubelet State")
			kubeletState, err = uniext.GetResourceUpdateNodeState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(kubeletState.Version).Should(gomega.Equal("1"))
			gomega.Expect(kubeletState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateSucceeded))

			ginkgo.By("Check CPUSet update status")
			resourceStatus, err = extension.GetResourceStatus(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err = cpuset.Parse(resourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(2))

			ginkgo.By("Initiate Third InplaceUpdate Request, foo/bar request update from 8 to 5")
			updatedContainerResource = newPod.Spec.Containers[0].Resources
			updatedContainerResource.Requests[newResourceName] = resource.MustParse("5")
			updatedContainerResource.Limits[newResourceName] = resource.MustParse("5")
			resourceUpdateSpec = &uniext.ResourceUpdateSpec{
				Version: "3",
				Containers: []uniext.ContainerResource{
					{
						Name:      pod.Spec.Containers[0].Name,
						Resources: updatedContainerResource,
					},
				},
			}

			f.PodClient().Update(pod.Name, func(pod *corev1.Pod) {
				resourceUpdateSpecData, err := json.Marshal(resourceUpdateSpec)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				pod.Annotations[uniext.AnnotationResourceUpdateSpec] = string(resourceUpdateSpecData)
			})

			ginkgo.By("Wait for Pod InplaceUpdate Complete")
			gomega.Eventually(func() bool {
				p, err := f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return !podHasUnhandledUpdateReq(p)
			}, 60*time.Second, 3*time.Second).Should(gomega.Equal(true))

			ginkgo.By("Check InplaceUpdate Scheduler State")
			newPod, err = f.PodClient().Get(context.TODO(), newPod.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			schedulerState, err = uniext.GetResourceUpdateSchedulerState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(schedulerState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(schedulerState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateAccepted))

			ginkgo.By("Check InplaceUpdate Kubelet State")
			kubeletState, err = uniext.GetResourceUpdateNodeState(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(kubeletState.Version).Should(gomega.Equal(resourceUpdateSpec.Version))
			gomega.Expect(kubeletState.Status).Should(gomega.Equal(uniext.ResourceUpdateStateSucceeded))

			ginkgo.By("Check Container Request and Limit")
			gomega.Expect(newPod.Spec.Containers[0].Resources).Should(gomega.Equal(updatedContainerResource))

			ginkgo.By("Check CPUSet update status")
			resourceStatus, err = extension.GetResourceStatus(newPod.Annotations)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(resourceStatus.CPUSet).NotTo(gomega.BeEmpty())
			cpuSet, err = cpuset.Parse(resourceStatus.CPUSet)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(cpuSet.Size()).To(gomega.Equal(2))
		})
	})
})

func podHasUnhandledUpdateReq(pod *corev1.Pod) bool {
	podAnnotations := pod.Annotations
	if podAnnotations == nil {
		podAnnotations = map[string]string{}
	}
	resourceUpdateSpec, err := uniext.GetResourceUpdateSpec(podAnnotations)
	if err != nil {
		return false
	}
	schedulerState, err := uniext.GetResourceUpdateSchedulerState(podAnnotations)
	if err != nil {
		return false
	}
	kubeletState, err := uniext.GetResourceUpdateNodeState(podAnnotations)
	if err != nil {
		return false
	}
	if (resourceUpdateSpec.Version == schedulerState.Version && resourceUpdateSpec.Version == kubeletState.Version) || (resourceUpdateSpec.Version == schedulerState.Version && schedulerState.Status == uniext.ResourceUpdateStateRejected) {
		return false
	}
	return true
}

func updateNode(nodeClient clientgov1.NodeInterface, nodeName string, updateFn func(node *corev1.Node)) {
	framework.ExpectNoError(wait.Poll(time.Millisecond*500, time.Second*30, func() (bool, error) {
		node, err := nodeClient.Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get node %q: %v", nodeName, err)
		}
		updateFn(node)
		_, err = nodeClient.UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
		if err == nil {
			klog.Infof("Successfully update node %s", nodeName)
			return true, nil
		}
		if apierrors.IsConflict(err) {
			klog.Infof("Conflicting update to node %q, re-get and re-update: %v", nodeName, err)
			return false, nil
		}
		return false, fmt.Errorf("failed to update node %q: %v", nodeName, err)
	}))
}
