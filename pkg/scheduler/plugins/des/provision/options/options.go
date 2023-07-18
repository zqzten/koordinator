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
