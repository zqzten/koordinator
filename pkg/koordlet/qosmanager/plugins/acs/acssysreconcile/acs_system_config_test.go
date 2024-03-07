package acssysreconcile

import (
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	mock_statesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
	sysutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util/cache"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

func Test_acsSystemConfig_reconcile(t *testing.T) {
	defaultACSSystemStrategy := &unified.ACSSystemStrategy{
		Enable: pointer.Bool(false),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: pointer.Int64(1),
			SchedAcpu:       pointer.Int64(0),
		},
	}
	disabledACSSystemStrategy := &unified.ACSSystemStrategy{
		Enable: pointer.Bool(false),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: pointer.Int64(1),
			SchedAcpu:       pointer.Int64(1),
		},
	}
	nilEnabledACSSystemStrategy := &unified.ACSSystemStrategy{
		Enable: pointer.Bool(true),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: nil,
			SchedAcpu:       nil,
		},
	}
	enabledACSSystemStrategy := &unified.ACSSystemStrategy{
		Enable: pointer.Bool(true),
		ACSSystem: unified.ACSSystem{
			SchedSchedStats: pointer.Int64(1),
			SchedAcpu:       pointer.Int64(0),
		},
	}
	tests := []struct {
		name         string
		initStrategy *unified.ACSSystemStrategy
		newStrategy  *unified.ACSSystemStrategy
		node         *corev1.Node
		expect       map[sysutil.Resource]string
	}{
		{
			name:         "testNodeNil",
			initStrategy: defaultACSSystemStrategy,
			newStrategy:  enabledACSSystemStrategy,
			expect: map[sysutil.Resource]string{
				sysutil.SchedSchedStats: strconv.FormatInt(*defaultACSSystemStrategy.SchedSchedStats, 10),
				sysutil.SchedAcpu:       strconv.FormatInt(*defaultACSSystemStrategy.SchedAcpu, 10),
			},
		},
		{
			name:         "testStrategyNil",
			initStrategy: defaultACSSystemStrategy,
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
			expect: map[sysutil.Resource]string{
				sysutil.SchedSchedStats: strconv.FormatInt(*defaultACSSystemStrategy.SchedSchedStats, 10),
				sysutil.SchedAcpu:       strconv.FormatInt(*defaultACSSystemStrategy.SchedAcpu, 10),
			},
		},
		{
			name:         "testStrategyDisabled",
			initStrategy: defaultACSSystemStrategy,
			newStrategy:  disabledACSSystemStrategy,
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
			expect: map[sysutil.Resource]string{
				sysutil.SchedSchedStats: strconv.FormatInt(*sloconfig.NoneACSSystemStrategy().SchedSchedStats, 10),
				sysutil.SchedAcpu:       strconv.FormatInt(*sloconfig.NoneACSSystemStrategy().SchedAcpu, 10),
			},
		},
		{
			name:         "testStrategyNilEnabled",
			initStrategy: defaultACSSystemStrategy,
			newStrategy:  nilEnabledACSSystemStrategy,
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
			expect: map[sysutil.Resource]string{
				sysutil.SchedSchedStats: strconv.FormatInt(*sloconfig.DefaultACSSystemStrategy().SchedSchedStats, 10),
				sysutil.SchedAcpu:       strconv.FormatInt(*sloconfig.DefaultACSSystemStrategy().SchedAcpu, 10),
			},
		},
		{
			name:         "testStrategyEnabled",
			initStrategy: defaultACSSystemStrategy,
			newStrategy:  enabledACSSystemStrategy,
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
				},
			},
			expect: map[sysutil.Resource]string{
				sysutil.SchedSchedStats: strconv.FormatInt(*enabledACSSystemStrategy.SchedSchedStats, 10),
				sysutil.SchedAcpu:       strconv.FormatInt(*enabledACSSystemStrategy.SchedAcpu, 10),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := sysutil.NewFileTestUtil(t)
			defer helper.Cleanup()
			prepareFiles(helper, tt.initStrategy)

			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mockstatesinformer := mock_statesinformer.NewMockStatesInformer(ctl)
			mockstatesinformer.EXPECT().GetNode().Return(tt.node).AnyTimes()
			mockstatesinformer.EXPECT().GetNodeSLO().Return(getNodeSLOByACSSystemStrategy(tt.newStrategy)).AnyTimes()

			reconcile := &systemConfig{
				statesInformer: mockstatesinformer,
				executor: &resourceexecutor.ResourceUpdateExecutorImpl{
					Config:        resourceexecutor.NewDefaultConfig(),
					ResourceCache: cache.NewCacheDefault(),
				},
			}
			stopCh := make(chan struct{})
			defer func() {
				close(stopCh)
			}()
			reconcile.executor.Run(stopCh)

			reconcile.reconcile()
			for file, expectValue := range tt.expect {
				got := helper.ReadFileContents(file.Path(""))
				assert.Equal(t, expectValue, got, file.Path(""))
			}
		})
	}
}

func prepareFiles(helper *sysutil.FileTestUtil, stragegy *unified.ACSSystemStrategy) {
	helper.CreateFile(sysutil.SchedSchedStats.Path(""))
	helper.WriteFileContents(sysutil.SchedSchedStats.Path(""), strconv.FormatInt(*stragegy.SchedSchedStats, 10))
	helper.CreateFile(sysutil.SchedAcpu.Path(""))
	helper.WriteFileContents(sysutil.SchedAcpu.Path(""), strconv.FormatInt(*stragegy.SchedAcpu, 10))
}

func getNodeSLOByACSSystemStrategy(strategy *unified.ACSSystemStrategy) *slov1alpha1.NodeSLO {
	strategyMap := map[string]interface{}{}
	if strategy != nil {
		if strategy.Enable != nil {
			strategyMap["enable"] = *strategy.Enable
		}
		if strategy.SchedSchedStats != nil {
			strategyMap["schedSchedStats"] = *strategy.SchedSchedStats
		}
		if strategy.SchedSchedStats != nil {
			strategyMap["schedAcpu"] = *strategy.SchedAcpu
		}
	}
	return &slov1alpha1.NodeSLO{
		Spec: slov1alpha1.NodeSLOSpec{
			Extensions: &slov1alpha1.ExtensionsMap{
				Object: map[string]interface{}{
					unified.ACSSystemExtKey: strategyMap,
				},
			},
		},
	}
}

func Test_systemConfig_Enabled(t *testing.T) {
	s := &systemConfig{
		reconcileInterval: time.Minute,
	}
	assert.Equalf(t, false, s.Enabled(), "Enabled()")
	err := features.DefaultMutableKoordletFeatureGate.SetFromMap(map[string]bool{string(features.ACSSystemConfig): true})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equalf(t, true, s.Enabled(), "Enabled()")
}
