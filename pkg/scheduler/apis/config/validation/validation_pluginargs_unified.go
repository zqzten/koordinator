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
