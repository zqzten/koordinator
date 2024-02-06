package logicalresourcenode

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/koordinator-sh/koordinator/pkg/util"
)

var (
	defaultConfig = (&Config{}).SetDefaults()
)

func TestGetConfig(t *testing.T) {
	cases := []struct {
		configMapData  map[string]string
		expectedConfig *Config
	}{
		{
			configMapData:  nil,
			expectedConfig: defaultConfig,
		},
		{
			configMapData:  map[string]string{},
			expectedConfig: defaultConfig,
		},
		{
			configMapData: map[string]string{
				"common": util.DumpJSON(CommonConfig{
					SyncNodeLabelKeys:                        []string{"foo"},
					SyncNodeConditionTypes:                   []string{"bar"},
					ReservationTerminationGracePeriodSeconds: 60,
					EnableQoSGroup:                           true,
				}),
			},
			expectedConfig: &Config{
				Common: CommonConfig{
					SyncNodeLabelKeys:                        append([]string{"foo"}, defaultSyncNodeLabelKeys...),
					SyncNodeConditionTypes:                   append([]string{"bar"}, defaultSyncNodeConditionTypes...),
					SkipSyncReservationLabelKeys:             defaultSkipSyncReservationLabelKeys,
					SyncReservationAnnotationKeys:            defaultSyncReservationAnnotationKeys,
					ReservationTerminationGracePeriodSeconds: 60,
					EnableQoSGroup:                           true,
				},
			},
		},
	}

	enableCache = false
	for i, tc := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			builder := fake.NewClientBuilder()
			if tc.configMapData != nil {
				builder.WithObjects(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "koordinator-system",
						Name:      configmapName,
					},
					Data: tc.configMapData,
				})
			}
			cli := builder.Build()

			gotConfig, err := GetConfig(cli)
			if err != nil {
				t.Fatal(err)
			}

			if !apiequality.Semantic.DeepEqual(gotConfig, tc.expectedConfig) {
				t.Fatalf("expected %v, got %v", util.DumpJSON(tc.expectedConfig), util.DumpJSON(gotConfig))
			}
		})
	}
}
