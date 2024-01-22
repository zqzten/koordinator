package plugins

import (
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/plugins/acs/acssysreconcile"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/plugins/acs/cpustable"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/plugins/acs/deadlineevict"
)

func init() {
	StrategyPlugins[deadlineevict.DeadlineEvictName] = deadlineevict.New
	StrategyPlugins[acssysreconcile.ACSSystemConfigReconcileName] = acssysreconcile.New
	StrategyPlugins[cpustable.PluginName] = cpustable.New
}
