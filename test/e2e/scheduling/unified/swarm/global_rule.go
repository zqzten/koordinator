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

package swarm

import (
	"context"
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
)

const GlobalRuleNS = "kube-system"
const ScheConfigMap = "unified-scheduler-config"
const GlobalRuleKey = "scheduler-config"

func createNamespaces(c clientset.Interface, ns string) (*v1.Namespace, error) {
	labels := map[string]string{}
	namespaceObj := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ns,
			Namespace: "",
			Labels:    labels,
		},
		Status: v1.NamespaceStatus{},
	}
	// Be robust about making the namespace creation call.
	var got *v1.Namespace
	if err := wait.PollImmediate(time.Second, 30*time.Second, func() (bool, error) {
		var err error
		got, err = c.CoreV1().Namespaces().Create(context.TODO(), namespaceObj, metav1.CreateOptions{})
		if err != nil {
			framework.Logf("Unexpected error while creating namespace: %v", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}
	return got, nil
}

func updateGlobalConfig(c clientset.Interface, key string, rule string) error {
	if _, err := c.CoreV1().Namespaces().Get(context.TODO(), GlobalRuleNS, metav1.GetOptions{}); err != nil {

		_, err := createNamespaces(c, GlobalRuleNS)
		if err != nil {
			return fmt.Errorf("failed to create ns %s, err %s", GlobalRuleNS, err)
		}
	}

	cf, err := c.CoreV1().ConfigMaps(GlobalRuleNS).Get(context.TODO(), ScheConfigMap, metav1.GetOptions{})
	if cf != nil && err == nil {
		if cf.Data == nil {
			cf.Data = make(map[string]string)
		}
		klog.Infof("update global rule key:%s to value:%s", key, rule)
		cf.Data[key] = rule
		_, err := c.CoreV1().ConfigMaps(GlobalRuleNS).Update(context.TODO(), cf, metav1.UpdateOptions{})
		return err
	} else if errors.IsNotFound(err) {
		ccf := v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: ScheConfigMap,
			},
			Data: map[string]string{
				key: rule,
			},
		}
		_, err := c.CoreV1().ConfigMaps(GlobalRuleNS).Create(context.TODO(), &ccf, metav1.CreateOptions{})
		return err
	}
	return err
}

// UpdateGlobalConfig update global rules
func UpdateGlobalConfig(c clientset.Interface, globalRule *GlobalRules) error {
	gr, _ := json.Marshal(globalRule)
	err := updateGlobalConfig(c, GlobalRuleKey, fmt.Sprintf("%s", gr[:]))
	if err != nil {
		return err
	}
	time.Sleep(120 * time.Second)
	return nil
}

// RemoveGlobalRule put app rules for mono app.
func RemoveGlobalRule(c clientset.Interface) error {
	// globalRule CAN NOT be deleted, scheduler process MUST NEED it.
	globalRule := &GlobalRules{
		UpdateTime: time.Now().Format(time.RFC3339),
	}

	return UpdateGlobalConfig(c, globalRule)
}
