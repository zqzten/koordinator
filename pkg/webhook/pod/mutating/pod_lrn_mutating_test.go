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

package mutating

import (
	"fmt"
	"testing"

	terwaytypes "github.com/AliyunContainerService/terway-apis/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInjectQoSGroupID(t *testing.T) {
	cases := []struct {
		groupID             string
		pod                 *corev1.Pod
		expectedPodNetworks string
	}{
		{
			groupID: "",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						terwaytypes.PodNetworksAnnotation: `{"podNetworks":[{"interface":"eth0","vSwitchOptions":["vsw-bp1e311x4lwgtp2zjfwoy"],"securityGroupIDs":["sg-bp12cxaffzv6baz3z0iq"],"userID":"1713704995379278","roleName":"aliyunpaidlcdefaultrole","extraRoutes":[{"dst":"10.10.10.10/32"}],"defaultRoute":true,"eniAvsConfig":{"queue":"4","ingressAclAccept":true}}]}`,
					},
				},
			},
			expectedPodNetworks: `{"podNetworks":[{"interface":"eth0","vSwitchOptions":["vsw-bp1e311x4lwgtp2zjfwoy"],"securityGroupIDs":["sg-bp12cxaffzv6baz3z0iq"],"userID":"1713704995379278","roleName":"aliyunpaidlcdefaultrole","extraRoutes":[{"dst":"10.10.10.10/32"}],"defaultRoute":true,"eniAvsConfig":{"queue":"4","ingressAclAccept":true}}]}`,
		},
		{
			groupID: "qg-123",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						terwaytypes.PodNetworksAnnotation: `{"podNetworks":[{"interface":"eth0","vSwitchOptions":["vsw-bp1e311x4lwgtp2zjfwoy"],"securityGroupIDs":["sg-bp12cxaffzv6baz3z0iq"],"userID":"1713704995379278","roleName":"aliyunpaidlcdefaultrole","extraRoutes":[{"dst":"10.10.10.10/32"}],"defaultRoute":true,"eniAvsConfig":{"queue":"4","ingressAclAccept":true}}]}`,
					},
				},
			},
			expectedPodNetworks: `{"podNetworks":[{"interface":"eth0","vSwitchOptions":["vsw-bp1e311x4lwgtp2zjfwoy"],"securityGroupIDs":["sg-bp12cxaffzv6baz3z0iq"],"userID":"1713704995379278","roleName":"aliyunpaidlcdefaultrole","extraRoutes":[{"dst":"10.10.10.10/32"}],"defaultRoute":true,"eniAvsConfig":{"queue":"4","ingressAclAccept":true},"eniQosGroupID":"qg-123"}]}`,
		},
	}

	for i, testCase := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			err := injectQoSGroupID(testCase.pod, testCase.groupID)
			if err != nil {
				t.Fatal(err)
			}
			require.JSONEq(t, testCase.expectedPodNetworks, testCase.pod.Annotations[terwaytypes.PodNetworksAnnotation])
		})
	}
}
