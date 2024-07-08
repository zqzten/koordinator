package intelligentscheduler

type IntelligentSchedulerRuntime struct {
	name        string
	scorePolicy string
}

func NewIntelligentSchedulerRuntime(name string, policy string) *IntelligentSchedulerRuntime {
	return &IntelligentSchedulerRuntime{
		name:        name,
		scorePolicy: policy,
	}
}
func (r *IntelligentSchedulerRuntime) Name() string {
	return r.name
}
func (r *IntelligentSchedulerRuntime) getScorePolicy() string {
	return r.scorePolicy
}

func (r *IntelligentSchedulerRuntime) Init() error {
	//TODO
	return nil
}
