package transformer

import (
	"github.com/spf13/pflag"
)

var enableENIResourceTransform bool
var enableTransformGPUCardRatio bool

func init() {
	pflag.BoolVar(&enableENIResourceTransform, "enable-eni-transform", enableENIResourceTransform, "enable eni resource transformer, disable by default")
	pflag.BoolVar(&enableTransformGPUCardRatio, "enable-transform-gpu-card-ratio", enableTransformGPUCardRatio, "enable transform pod gpu card ratio, disable by default")
}
