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
	"github.com/spf13/pflag"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
)

var (
	DefaultECIProfile = &ECIProfile{
		DefaultStorageClass: "alicloud-disk-efficiency",
		AllowedAffinityKeys: []string{
			uniext.LabelCommonNodeType,
			uniext.LabelNodeType,
			corev1.LabelTopologyZone,
			corev1.LabelZoneFailureDomain,
			"sigma.ali/ecs-zone-id",
		},
	}
)

type ECIProfile struct {
	DefaultStorageClass string
	AllowedAffinityKeys []string
}

func init() {
	pflag.StringSliceVar(&DefaultECIProfile.AllowedAffinityKeys, "eci-profile-allowed-affinity-keys", DefaultECIProfile.AllowedAffinityKeys, "--eci-profile-allowed-affinity-key=[key1, key2, ...]")
	pflag.StringVar(&DefaultECIProfile.DefaultStorageClass, "eci-profile-default-storage-class", DefaultECIProfile.DefaultStorageClass, "--eci-profile-default-storage-class=your-storageClass")
}
