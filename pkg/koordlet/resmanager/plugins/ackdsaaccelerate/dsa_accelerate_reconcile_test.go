package ackdsa

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
)

func createNodeSLO() *slov1alpha1.NodeSLO {
	nodeSLO := &slov1alpha1.NodeSLO{
		TypeMeta: metav1.TypeMeta{Kind: "NodeSLO"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test_nodeslo",
			UID:         "test_nodeslo",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	return nodeSLO
}

//nodeSLO.Spec.Extensions.Object
func Test_parseDsaAccelerateSpec(t *testing.T) {

	testingDsaStrategy1 := &ackapis.DsaAccelerateStrategy{Enable: pointer.BoolPtr(true)}
	testingObject1 := make(map[string]interface{})
	testingObject1[ackapis.DsaAccelerateExtKey] = testingDsaStrategy1
	testingExtensions1 := &slov1alpha1.ExtensionsMap{
		Object: testingObject1,
	}
	testingNodeSLO1 := createNodeSLO()
	testingNodeSLO1.Spec = slov1alpha1.NodeSLOSpec{
		Extensions: testingExtensions1,
	}

	testingDsaConfig2 := ackapis.DsaAccelerateConfig{PollingMode: pointer.BoolPtr(false)}
	testingDsaStrategy2 := &ackapis.DsaAccelerateStrategy{Enable: pointer.BoolPtr(true), DsaAccelerateConfig: testingDsaConfig2}
	testingObject2 := make(map[string]interface{})
	testingObject2[ackapis.DsaAccelerateExtKey] = testingDsaStrategy2
	testingExtensions2 := &slov1alpha1.ExtensionsMap{
		Object: testingObject2,
	}
	testingNodeSLO2 := createNodeSLO()
	testingNodeSLO2.Spec = slov1alpha1.NodeSLOSpec{
		Extensions: testingExtensions2,
	}

	type agrs struct {
		name    string
		nodeSLO *slov1alpha1.NodeSLO
		want    *ackapis.DsaAccelerateStrategy
		err     bool
	}

	tests := []agrs{

		{
			name:    "polling mode is nil",
			nodeSLO: testingNodeSLO1,
			want:    testingDsaStrategy1,
			err:     false,
		},
		{
			name:    "correct dsa strategy",
			nodeSLO: testingNodeSLO2,
			want:    testingDsaStrategy2,
			err:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDsaAccelerateSpec(tt.nodeSLO)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
