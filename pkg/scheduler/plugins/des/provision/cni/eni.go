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

package cni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	invsdk "gitlab.alibaba-inc.com/dbpaas/Inventory/inventory-sdk-go/sdk"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/des/provision/options"
)

type NetworkInterfaceProvisioner struct {
	kubeClient  kubernetes.Interface
	classLister storagelisters.StorageClassLister
	podLister   corelisters.PodLister
	nodeLister  corelisters.NodeLister
	pvcLister   corelisters.PersistentVolumeClaimLister
	pvLister    corelisters.PersistentVolumeLister
	invClient   invsdk.Client
	accountMap  *corev1.ConfigMap
}

func NewNetworkInterfaceProvisioner(kubeClient kubernetes.Interface,
	podInformer coreinformers.PodInformer,
	nodeInformer coreinformers.NodeInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	pvInformer coreinformers.PersistentVolumeInformer,
	storageClassInformer storageinformers.StorageClassInformer,
	invClient invsdk.Client,
	accountMap *corev1.ConfigMap) ReserveNetworkInterfaceProvisioner {
	return &NetworkInterfaceProvisioner{
		kubeClient:  kubeClient,
		podLister:   podInformer.Lister(),
		nodeLister:  nodeInformer.Lister(),
		pvcLister:   pvcInformer.Lister(),
		pvLister:    pvInformer.Lister(),
		classLister: storageClassInformer.Lister(),
		invClient:   invClient,
		accountMap:  accountMap,
	}
}

func (np *NetworkInterfaceProvisioner) CreateEni(pod *corev1.Pod, node *corev1.Node) (map[string]invsdk.CreateEniInDesResponse, error) {
	regionId := node.Labels[options.RegionIdLabel]
	zoneId := node.Labels[options.ZoneLabel]
	vswId := node.Labels[options.VSWIdLabel]
	serviceAccountVSwitchId := ""
	sgId := node.Labels[options.SecurityGroupIdLabel]
	serviceAccountSgId := ""
	engine := pod.Labels[options.InstanceEngineLabel]
	accountId := np.accountMap.Data[options.AccountMapUID]
	account := np.accountMap.Data[options.AccountMapAccount]
	serviceAccount := ""

	interfaceCustomConfig := make(map[string]InterfaceCustomConfig)
	if inputConfig := pod.Annotations[options.InterfaceCustomConfig]; inputConfig != "" {
		if err := json.Unmarshal([]byte(inputConfig), &interfaceCustomConfig); err != nil {
			return nil, fmt.Errorf("unmarshal interfaceCustomConfig[%s] failed: %v", inputConfig, err)
		}
		serviceAccountSgId = np.accountMap.Data[options.AccountMapSecurityGroup]

		if v, ok := np.accountMap.Data[options.AccountMapVSwitches]; ok {
			vswitches := make(map[string]map[string]string)
			if err := json.Unmarshal([]byte(v), &vswitches); err != nil {
				return nil, fmt.Errorf("unmarshal vswitches failed: %v", err)
			}
			serviceAccountVSwitchId = vswitches[zoneId]["vswitch"]
		}
		serviceAccount = np.accountMap.Data[options.AccountAliGroupRamRole]
	}

	enis := make(map[string]invsdk.CreateEniInDesResponse)
	if len(interfaceCustomConfig) == 0 {
		req := invsdk.CreateEniInDesRequest{
			RequestId:       options.GenerateRequestId(),
			RegionId:        regionId,
			Name:            pod.Name,
			Engine:          engine,
			VswitchId:       vswId,
			SecurityGroupId: sgId,
			NodeSn:          node.Labels[options.EcsInstanceIdLabel],
			BizType:         np.accountMap.Data[options.AccountMapBizType],
			Account:         account,
			AccountUserId:   np.accountMap.Data[options.AccountMapUID],
			PodName:         pod.Name,
		}
		if pod.Labels[options.UseTrunkENILabel] == "true" {
			req.TrunkEniId = node.Annotations[options.TrunkENIInterfaceIdAnnotation]
		}
		resp, err := np.invClient.CreateEniInDes(req)
		if err == nil {
			enis[np.accountMap.Data[options.AccountMapUID]] = resp
		} else {
			return enis, err
		}
	} else {
		for _, customConfig := range interfaceCustomConfig {
			vswitchId := vswId
			securityCgroupId := sgId
			at := account
			if customConfig.ServiceAccountUID != accountId {
				vswitchId = serviceAccountVSwitchId
				securityCgroupId = serviceAccountSgId
				at = serviceAccount
			}
			req := invsdk.CreateEniInDesRequest{
				RequestId:       options.GenerateRequestId(),
				RegionId:        regionId,
				Name:            pod.Name,
				Engine:          engine,
				VswitchId:       vswitchId,
				SecurityGroupId: securityCgroupId,
				NodeSn:          node.Labels[options.EcsInstanceIdLabel],
				BizType:         np.accountMap.Data[options.AccountMapBizType],
				Account:         at,
				AccountUserId:   customConfig.ServiceAccountUID,
				PodName:         pod.Name,
			}
			if pod.Labels[options.UseTrunkENILabel] == "true" {
				req.TrunkEniId = node.Annotations[options.TrunkENIInterfaceIdAnnotation]
			}
			resp, err := np.invClient.CreateEniInDes(req)
			if err == nil {
				enis[customConfig.ServiceAccountUID] = resp
			} else {
				return enis, err
			}
		}
	}

	return enis, nil
}

func DeleteNetworkInterface(pod *corev1.Pod, networkInterfaceId string, invClient invsdk.Client) error {
	if pod == nil {
		klog.Infof("pod is nil, skip")
		return nil
	}

	if networkInterfaceId == "" {
		eniInfoStr := pod.Annotations[options.AsiEniAnnotation]
		if pod.Labels[options.UseTrunkENILabel] != "true" || eniInfoStr == "" {
			return nil
		}
		var injectedNetworkInterfaces []*InjectedNetworkInterface
		err := json.Unmarshal([]byte(eniInfoStr), &injectedNetworkInterfaces)
		if err != nil {
			klog.Errorf("error unmarshal injectedNetworkInterfaces: %v", err)
			return err
		}
		networkInterfaceId = injectedNetworkInterfaces[0].NetworkInterfaceId
	}
	truncENIId := pod.Annotations[options.TrunkENIInterfaceIdAnnotation]
	req := invsdk.DeleteEniInDesRequest{
		RequestId:          options.GenerateRequestId(),
		Engine:             pod.Labels[options.InstanceEngineLabel],
		NetworkInterfaceId: networkInterfaceId,
	}

	_, err := invClient.DeleteEniInDes(req)
	if err != nil {
		klog.Errorf("failed to delete eni %s(%s), err: %v", networkInterfaceId, truncENIId, err)
		return err
	}

	return nil
}

func (np *NetworkInterfaceProvisioner) DeleteEni(ctx context.Context, pod *corev1.Pod, networkInterfaceId string) error {
	return DeleteNetworkInterface(pod, networkInterfaceId, np.invClient)
}

func (np *NetworkInterfaceProvisioner) PatchEni(ctx context.Context, pod *corev1.Pod, node *corev1.Node, interfaceSet map[string]invsdk.CreateEniInDesResponse) error {
	regionId := node.Labels[options.RegionIdLabel]
	zoneId := node.Labels[options.ZoneLabel]
	accountId := np.accountMap.Data[options.AccountMapUID]
	vSwitchCIDR := node.Annotations[options.VSWCIDRLabel]
	serviceAccountVSwitchCIDR := ""
	interfaceCustomConfig := make(map[string]InterfaceCustomConfig)
	if inputConfig := pod.Annotations[options.InterfaceCustomConfig]; inputConfig != "" {
		if err := json.Unmarshal([]byte(inputConfig), &interfaceCustomConfig); err != nil {
			return fmt.Errorf("unmarshal interfaceCustomConfig[%s] failed: %v", inputConfig, err)
		}
	}
	if v, ok := np.accountMap.Data[options.AccountMapVSwitches]; ok {
		vswitches := make(map[string]map[string]string)
		if err := json.Unmarshal([]byte(v), &vswitches); err != nil {
			return fmt.Errorf("unmarshal vswitches failed: %v", err)
		}
		serviceAccountVSwitchCIDR = vswitches[zoneId]["cidr"]
	}

	var injectedNetworkInterfaces []*InjectedNetworkInterface
	if len(interfaceCustomConfig) == 0 {
		injectedNetworkInterfaces = append(injectedNetworkInterfaces, &InjectedNetworkInterface{
			Provider:            "apsaradb",
			CloudAccountId:      accountId,
			VSwitchCidrBlock:    vSwitchCIDR,
			MacAddress:          interfaceSet[accountId].MacAddress,
			VSwitchId:           interfaceSet[accountId].VSwitchId,
			ResourceType:        "eni",
			Ipv4:                interfaceSet[accountId].PrivateIpAddress,
			NetworkInterfaceId:  interfaceSet[accountId].NetworkInterfaceId,
			RegionId:            regionId,
			ZoneId:              zoneId,
			VpcId:               interfaceSet[accountId].VpcId,
			SecurityGroupIdList: interfaceSet[accountId].SecurityGroupIds.SecurityGroupId,
			NeedRamRole:         true,
		})
	} else {
		for iface, customConfig := range interfaceCustomConfig {
			cidr := vSwitchCIDR
			needRamRole := true
			if customConfig.ServiceAccountUID != accountId {
				cidr = serviceAccountVSwitchCIDR
				needRamRole = false
			}
			injectedNetworkInterfaces = append(injectedNetworkInterfaces, &InjectedNetworkInterface{
				Interface:           iface,
				Provider:            "apsaradb",
				CloudAccountId:      customConfig.ServiceAccountUID,
				VSwitchCidrBlock:    cidr,
				MacAddress:          interfaceSet[customConfig.ServiceAccountUID].MacAddress,
				VSwitchId:           interfaceSet[customConfig.ServiceAccountUID].VSwitchId,
				ResourceType:        "eni",
				Ipv4:                interfaceSet[customConfig.ServiceAccountUID].PrivateIpAddress,
				NetworkInterfaceId:  interfaceSet[customConfig.ServiceAccountUID].NetworkInterfaceId,
				RegionId:            regionId,
				ZoneId:              zoneId,
				VpcId:               interfaceSet[customConfig.ServiceAccountUID].VpcId,
				SecurityGroupIdList: interfaceSet[customConfig.ServiceAccountUID].SecurityGroupIds.SecurityGroupId,
				DefaultRoute:        customConfig.DefaultRoute,
				ExtraRoutes:         customConfig.ExtraRoutes,
				NeedRamRole:         needRamRole,
			})
		}
	}

	if pod.Labels[options.UseTrunkENILabel] == "true" {
		for i, ifs := range injectedNetworkInterfaces {
			injectedNetworkInterfaces[i].MacAddress = node.Annotations[options.TrunkENIInterfaceMacAnnotation]
			injectedNetworkInterfaces[i].VlanId = interfaceSet[ifs.CloudAccountId].Attachment.DeviceIndex
			injectedNetworkInterfaces[i].ResourceType = "member-eni"
		}

	}

	// Step3: annotate node
	truncENIId := node.Annotations[options.TrunkENIInterfaceIdAnnotation]
	b, err := json.Marshal(&injectedNetworkInterfaces)
	if err != nil {
		return fmt.Errorf("marshal eni annotations failed: %v", err)
	}
	buf := bytes.NewBuffer(b)
	patchData := map[string]interface{}{
		"metadata": map[string]map[string]string{
			"annotations": {options.AsiEniAnnotation: buf.String(), options.AsiTenancyKey: "true", options.TrunkENIInterfaceIdAnnotation: truncENIId, options.RegionIdLabel: regionId},
		},
	}
	// node eni category maybe changed, so we will patch it each period use types.StrategicMergePatchType
	patch, err := json.Marshal(&patchData)
	if err != nil {
		return fmt.Errorf("failed to serialize patchDataRaw: %v", err)
	}
	if _, err = np.kubeClient.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed to patch eni annotation: %v", err)
	}

	return nil
}
