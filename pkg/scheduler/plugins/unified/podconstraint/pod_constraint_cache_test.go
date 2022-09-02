package podconstraint

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func TestPodConstraintCache_AddConstraint(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-2",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na620",
					corev1.LabelHostname:     "test-node-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-3",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-3",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-4",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na630",
					corev1.LabelHostname:     "test-node-4",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	//
	// constructs topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  2    |  2    |  2    |
	//  +-------+-------+-------+
	//
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
	}
	pod.Name = "pod-1"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-2"
	plg.podConstraintCache.AddPod(nodes[0], pod)
	pod.Name = "pod-3"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-4"
	plg.podConstraintCache.AddPod(nodes[1], pod)
	pod.Name = "pod-5"
	plg.podConstraintCache.AddPod(nodes[2], pod)
	pod.Name = "pod-6"
	plg.podConstraintCache.AddPod(nodes[2], pod)

	gotState := plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 6,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 2,
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na620"}: 2,
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na630"}: 2,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 2, TopologyValue: "na630"},
				Max: CriticalPath{MatchNum: 2, TopologyValue: "na620"},
			},
		},
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_AddConstraintWithNewConstraint(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	var ch = make(chan int)
	suit.Handle.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ch <- 1
		},
	})
	suit.Handle.SharedInformerFactory().Start(context.TODO().Done())
	suit.Handle.SharedInformerFactory().WaitForCacheSync(context.TODO().Done())
	<-ch

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodes[0].Name,
		},
	}
	plg.podConstraintCache.AddPod(nodes[0], pod)

	gotState := plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)

	podConstraint.Spec.SpreadRule.Requires = append(podConstraint.Spec.SpreadRule.Requires, v1beta1.SpreadRuleItem{
		TopologyKey: corev1.LabelHostname,
		MaxSkew:     1,
	})
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	gotState = plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState = &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
			{
				TopologyKey: corev1.LabelHostname,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
			corev1.LabelHostname:     1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   1,
			{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
			corev1.LabelHostname: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "test-node-1"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_AddConstraintWithDeleteOneConstraint(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					corev1.LabelTopologyZone: "na610",
					corev1.LabelHostname:     "test-node-1",
				},
			},
		},
	}
	suit := newPluginTestSuit(t, nodes)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	for _, node := range nodes {
		_, err := suit.Handle.ClientSet().CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
		assert.NoError(t, err)
	}
	var ch = make(chan int)
	suit.Handle.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ch <- 1
		},
	})
	suit.Handle.SharedInformerFactory().Start(context.TODO().Done())
	suit.Handle.SharedInformerFactory().WaitForCacheSync(context.TODO().Done())
	<-ch

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
					{
						TopologyKey: corev1.LabelHostname,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-pod",
			Labels: map[string]string{
				extunified.LabelPodConstraint: podConstraint.Name,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodes[0].Name,
		},
	}
	plg.podConstraintCache.AddPod(nodes[0], pod)

	gotState := plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
			{
				TopologyKey: corev1.LabelHostname,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
			corev1.LabelHostname:     1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}:   1,
			{TopologyKey: corev1.LabelHostname, TopologyValue: "test-node-1"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
			corev1.LabelHostname: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "test-node-1"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)

	podConstraint = &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	gotState = plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState = &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{
			corev1.LabelTopologyZone: 1,
		},
		TpPairToMatchNum: map[TopologyPair]int{
			{TopologyKey: corev1.LabelTopologyZone, TopologyValue: "na610"}: 1,
		},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: 1, TopologyValue: "na610"},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)
}

func TestPodConstraintCache_DelPodConstraint(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey: corev1.LabelTopologyZone,
						MaxSkew:     1,
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	gotState := plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{},
		TpPairToMatchNum:     map[TopologyPair]int{},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""}},
		},
	}
	assert.Equal(t, expectState, gotState)

	plg.podConstraintCache.DelPodConstraint(podConstraint)

	gotState = plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	assert.Nil(t, gotState)
}

func TestConstraintTopologyValue(t *testing.T) {
	suit := newPluginTestSuit(t, nil)
	p, err := suit.proxyNew(suit.args, suit.Handle)
	assert.NotNil(t, p)
	assert.Nil(t, err)

	plg := p.(*Plugin)
	suit.start()

	//
	// constructs PodConstraint with TopologyRatio
	// topology: topology.kubernetes.io/zone
	//  +-------+-------+-------+
	//  | na610 | na620 | na630 |
	//  |-------+-------+-------|
	//  |  20%  |  20%  |  60%  |
	//  +-------+-------+-------+
	//
	podConstraint := &v1beta1.PodConstraint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: v1beta1.PodConstraintSpec{
			SpreadRule: v1beta1.SpreadRule{
				Requires: []v1beta1.SpreadRuleItem{
					{
						TopologyKey:   corev1.LabelTopologyZone,
						MaxSkew:       1,
						PodSpreadType: v1beta1.PodSpreadTypeRatio,
						TopologyRatios: []v1beta1.TopologyRatio{
							{
								TopologyValue: "na610",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na620",
								Ratio:         pointer.Int32Ptr(2),
							},
							{
								TopologyValue: "na630",
								Ratio:         pointer.Int32Ptr(6),
							},
						},
					},
				},
			},
		},
	}
	plg.podConstraintCache.SetPodConstraint(podConstraint)

	gotState := plg.podConstraintCache.GetState(getNamespacedName(podConstraint.Namespace, podConstraint.Name))
	expectState := &TopologySpreadConstraintState{
		PodConstraint: podConstraint,
		RequiredSpreadConstraints: []*TopologySpreadConstraint{
			{
				TopologyKey: corev1.LabelTopologyZone,
				MaxSkew:     1,
				TopologyRatios: map[string]int{
					"na610": 2,
					"na620": 2,
					"na630": 6,
				},
				TopologySumRatio: 10,
			},
		},
		TpKeyToTotalMatchNum: map[string]int{},
		TpPairToMatchNum:     map[TopologyPair]int{},
		TpKeyToCriticalPaths: map[string]*TopologyCriticalPaths{
			corev1.LabelTopologyZone: {
				Min: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
				Max: CriticalPath{MatchNum: math.MaxInt32, TopologyValue: ""},
			},
		},
	}
	assert.Equal(t, expectState, gotState)
}
