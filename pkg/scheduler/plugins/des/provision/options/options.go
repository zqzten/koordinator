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

package options

import (
	"fmt"

	uuid "github.com/satori/go.uuid"
)

const (
	ZoneLabel            = "topology.kubernetes.io/zone"
	RegionIdLabel        = "rm.alibaba-inc.com/region"
	EcsInstanceIdLabel   = "rm.alibaba-inc.com/ecsInstanceId"
	SecurityGroupIdLabel = "rm.alibaba-inc.com/securityGroupId"
	VSWIdLabel           = "rm.alibaba-inc.com/vswitchId"
	VSWCIDRLabel         = "rm.alibaba-inc.com/vswitchCidr"

	AsiEniAnnotation = "alibabacloud.com/injected-network-interfaces"
	AsiTenancyKey    = "tenancy.x-k8s.io/cluster"

	TrunkENIInterfaceIdAnnotation  = "rm.alibaba-inc.com/eniTrunkInterfaceID"
	TrunkENIInterfaceMacAnnotation = "rm.alibaba-inc.com/eniTrunkInterfaceMac"
	EnableECILabel                 = "alibabacloud.com/eci"
	InstanceEngineLabel            = "db.alicloud.io/instance-engine"
	UseTrunkENILabel               = "db.alicloud.io/use-trunk-eni"
	UseENILabel                    = "db.alicloud.io/use-eni"
	InterfaceCustomConfig          = "db.alicloud.io/interface-custom-config"
)

const (
	InventoryAccountName    = "inventory-account-map"
	AccountMapAccount       = "account"
	AccountMapUID           = "uid"
	AccountMapBizType       = "biztype"
	AccountMapVSwitches     = "aligroupVswitches"
	AccountMapSecurityGroup = "aliGroupsecurityGroup"
	AccountAliGroupRamRole  = "aliGroupramRole"
)

func GenerateRequestId() string {
	return fmt.Sprintf("%s-%s-%s", uuid.NewV4().String()[0:6], uuid.NewV4().String()[0:4], uuid.NewV4().String()[0:8])
}
