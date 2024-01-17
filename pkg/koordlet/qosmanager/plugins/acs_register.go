package plugins

import "github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/plugins/acs/deadlineevict"

func init() {
	StrategyPlugins[deadlineevict.DeadlineEvictName] = deadlineevict.New
}
