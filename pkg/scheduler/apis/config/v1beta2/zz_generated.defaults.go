//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by defaulter-gen. DO NOT EDIT.

package v1beta2

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// RegisterDefaults adds defaulters functions to the given scheme.
// Public to allow building arbitrary schemes.
// All generated defaulters are covering - they call all nested defaulters.
func RegisterDefaults(scheme *runtime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&CachedPodArgs{}, func(obj interface{}) { SetObjectDefaults_CachedPodArgs(obj.(*CachedPodArgs)) })
	scheme.AddTypeDefaultingFunc(&CoschedulingArgs{}, func(obj interface{}) { SetObjectDefaults_CoschedulingArgs(obj.(*CoschedulingArgs)) })
	scheme.AddTypeDefaultingFunc(&DeviceShareArgs{}, func(obj interface{}) { SetObjectDefaults_DeviceShareArgs(obj.(*DeviceShareArgs)) })
	scheme.AddTypeDefaultingFunc(&ElasticQuotaArgs{}, func(obj interface{}) { SetObjectDefaults_ElasticQuotaArgs(obj.(*ElasticQuotaArgs)) })
	scheme.AddTypeDefaultingFunc(&LoadAwareSchedulingArgs{}, func(obj interface{}) { SetObjectDefaults_LoadAwareSchedulingArgs(obj.(*LoadAwareSchedulingArgs)) })
	scheme.AddTypeDefaultingFunc(&NodeNUMAResourceArgs{}, func(obj interface{}) { SetObjectDefaults_NodeNUMAResourceArgs(obj.(*NodeNUMAResourceArgs)) })
	scheme.AddTypeDefaultingFunc(&ReservationArgs{}, func(obj interface{}) { SetObjectDefaults_ReservationArgs(obj.(*ReservationArgs)) })
	scheme.AddTypeDefaultingFunc(&UnifiedPodConstraintArgs{}, func(obj interface{}) { SetObjectDefaults_UnifiedPodConstraintArgs(obj.(*UnifiedPodConstraintArgs)) })
	return nil
}

func SetObjectDefaults_CachedPodArgs(in *CachedPodArgs) {
	SetDefaults_CachedPodArgs(in)
}

func SetObjectDefaults_CoschedulingArgs(in *CoschedulingArgs) {
	SetDefaults_CoschedulingArgs(in)
}

func SetObjectDefaults_DeviceShareArgs(in *DeviceShareArgs) {
	SetDefaults_DeviceShareArgs(in)
}

func SetObjectDefaults_ElasticQuotaArgs(in *ElasticQuotaArgs) {
	SetDefaults_ElasticQuotaArgs(in)
}

func SetObjectDefaults_LoadAwareSchedulingArgs(in *LoadAwareSchedulingArgs) {
	SetDefaults_LoadAwareSchedulingArgs(in)
}

func SetObjectDefaults_NodeNUMAResourceArgs(in *NodeNUMAResourceArgs) {
	SetDefaults_NodeNUMAResourceArgs(in)
}

func SetObjectDefaults_ReservationArgs(in *ReservationArgs) {
	SetDefaults_ReservationArgs(in)
}

func SetObjectDefaults_UnifiedPodConstraintArgs(in *UnifiedPodConstraintArgs) {
	SetDefaults_UnifiedPodConstraintArgs(in)
}
