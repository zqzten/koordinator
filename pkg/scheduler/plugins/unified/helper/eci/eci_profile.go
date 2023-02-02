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
