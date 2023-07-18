package cni

import (
	"context"

	invsdk "gitlab.alibaba-inc.com/dbpaas/Inventory/inventory-sdk-go/sdk"

	corev1 "k8s.io/api/core/v1"
)

type InjectedNetworkInterface struct {
	Interface           string   `json:"interface,omitempty"`
	Provider            string   `json:"provider,omitempty"`
	CloudAccountId      string   `json:"cloudAccountId,omitempty"`
	VSwitchCidrBlock    string   `json:"vSwitchCidrBlock,omitempty"`
	MacAddress          string   `json:"macAddress,omitempty"`
	VSwitchId           string   `json:"vSwitchId,omitempty"`
	NetworkInterfaceId  string   `json:"networkInterfaceId,omitempty"`
	Ipv4                string   `json:"ipv4,omitempty"`
	RegionId            string   `json:"regionId,omitempty"`
	ZoneId              string   `json:"zoneId,omitempty"`
	VpcId               string   `json:"vpcId,omitempty"`
	SecurityGroupIdList []string `json:"securityGroupIdList,omitempty"`
	ResourceType        string   `json:"resourceType,omitempty"`
	VlanId              int      `json:"vlanId,omitempty"`
	DefaultRoute        bool     `json:"defaultRoute,omitempty"`
	ExtraRoutes         []Route  `json:"extraRoutes,omitempty"`
	NeedRamRole         bool     `json:"needRamRole,omitempty"`
}

type Route struct {
	Dst string `json:"dst,omitempty"`
}

type InterfaceCustomConfig struct {
	ServiceAccountUID string  `json:"serviceAccountUID,omitempty"`
	DefaultRoute      bool    `json:"defaultRoute,omitempty"`
	ExtraRoutes       []Route `json:"extraRoutes,omitempty"`
}

type ReserveNetworkInterfaceProvisioner interface {
	CreateEni(pod *corev1.Pod, node *corev1.Node) (map[string]invsdk.CreateEniInDesResponse, error)
	PatchEni(ctx context.Context, pod *corev1.Pod, node *corev1.Node, interfaceSet map[string]invsdk.CreateEniInDesResponse) error
	DeleteEni(ctx context.Context, pod *corev1.Pod, networkInterfaceId string) error
}
