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

package validation

import (
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
)

func ValidateLimitAwareArgs(limitAwareArgs *config.LimitAwareArgs) error {
	var allErrs field.ErrorList
	if err := validateResourceWeights(limitAwareArgs.ScoringResourceWeights); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("scoringResourceWeights"), limitAwareArgs.ScoringResourceWeights, err.Error()))
	}
	return allErrs.ToAggregate()
}

func ValidateASIQuotaAdaptorArgs(args *config.ASIQuotaAdaptorArgs) error {
	var allErrs field.ErrorList
	rangeConfPath := field.NewPath("priorityRangeConfig")
	for k, v := range args.PriorityRangeConfig {
		path := rangeConfPath.Key(k)
		allErrs = append(allErrs, validatePriorityRangeConf(path, v)...)
	}
	if len(allErrs) == 0 {
		return nil
	}
	return allErrs.ToAggregate()
}

func validatePriorityRangeConf(path *field.Path, conf *config.PriorityRangeConfig) field.ErrorList {
	allErrs := field.ErrorList{}
	if conf.PriorityStart > conf.PriorityEnd {
		allErrs = append(allErrs, field.Invalid(path.Child("priorityStart"), conf.PriorityStart, "must be equal or bigger than priorityEnd"))
	}
	return allErrs
}
