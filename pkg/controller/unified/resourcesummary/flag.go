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

package resourcesummary

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
)

var (
	overSellPercents = &OverSellPercents{
		percents: map[corev1.ResourceName]int64{
			corev1.ResourceCPU:    10000,
			corev1.ResourceMemory: 100,
			uniext.ResourceACU:    10000,
		},
	}
	_ flag.Value = &OverSellPercents{}
)

type OverSellPercents struct {
	percents map[corev1.ResourceName]int64
}

func (o *OverSellPercents) String() string {
	s := strings.Builder{}
	for resourceName, percent := range o.percents {
		s.WriteString(fmt.Sprintf("%s:%d,", resourceName, percent))
	}
	return s.String()
}

func (o *OverSellPercents) Set(s string) error {
	// --over-sell-percents=cpu:10000,memory:100,acu:10000
	rawPercents := strings.Split(s, ",")
	for _, rawPercent := range rawPercents {
		parts := strings.Split(rawPercent, ":")
		if len(parts) != 2 {
			return fmt.Errorf("expect resourceName:percent, but got %v", rawPercents)
		}
		resourceName := parts[0]
		percent, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return err
		}
		o.percents[corev1.ResourceName(resourceName)] = percent
	}
	return nil
}

func InitFlags(fs *flag.FlagSet) {
	fs.Var(overSellPercents, "over-sell-percents", "--over-sell-percents=(resource-name:percent,)*")

}
