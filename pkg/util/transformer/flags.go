package transformer

import (
	"github.com/spf13/pflag"
)

var enableENIResourceTransform bool

func init() {
	pflag.BoolVar(&enableENIResourceTransform, "enable-eni-transform", enableENIResourceTransform, "enable eni resource transformer, disable by default")
}
